package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Server struct {
	clientset *kubernetes.Clientset
}

type PodInfo struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	NodeName  string `json:"nodeName"`
}

func NewServer() (*Server, error) {
	config, err := getKubernetesConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	return &Server{clientset: clientset}, nil
}

func getKubernetesConfig() (*rest.Config, error) {
	// Try in-cluster config first (when running in pod)
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	// Fallback to kubeconfig file (for local development)
	kubeconfig := filepath.Join(os.Getenv("HOME"), ".kube", "config")
	if envKubeconfig := os.Getenv("KUBECONFIG"); envKubeconfig != "" {
		kubeconfig = envKubeconfig
	}

	return clientcmd.BuildConfigFromFlags("", kubeconfig)
}

func (s *Server) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("OK"))
}

func (s *Server) readyHandler(w http.ResponseWriter, r *http.Request) {
	// Check if we can connect to Kubernetes API
	_, err := s.clientset.CoreV1().Namespaces().List(context.TODO(), metav1.ListOptions{Limit: 1})
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		w.Write([]byte("Not Ready"))
		return
	}
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("Ready"))
}

func (s *Server) listPodsHandler(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	if namespace == "" {
		namespace = "default"
	}

	pods, err := s.clientset.CoreV1().Pods(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to list pods: %v", err), http.StatusInternalServerError)
		return
	}

	var podInfos []PodInfo
	for _, pod := range pods.Items {
		podInfo := PodInfo{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Status:    string(pod.Status.Phase),
			NodeName:  pod.Spec.NodeName,
		}
		podInfos = append(podInfos, podInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(podInfos)
}

func (s *Server) getPodHandler(w http.ResponseWriter, r *http.Request) {
	namespace := r.URL.Query().Get("namespace")
	podName := r.URL.Query().Get("name")

	if namespace == "" {
		namespace = "default"
	}

	if podName == "" {
		http.Error(w, "Pod name is required", http.StatusBadRequest)
		return
	}

	pod, err := s.clientset.CoreV1().Pods(namespace).Get(context.TODO(), podName, metav1.GetOptions{})
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to get pod: %v", err), http.StatusNotFound)
		return
	}

	podInfo := PodInfo{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Status:    string(pod.Status.Phase),
		NodeName:  pod.Spec.NodeName,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(podInfo)
}

func main() {
	server, err := NewServer()
	if err != nil {
		log.Fatalf("Failed to create server: %v", err)
	}

	http.HandleFunc("/health", server.healthHandler)
	http.HandleFunc("/ready", server.readyHandler)
	http.HandleFunc("/pods", server.listPodsHandler)
	http.HandleFunc("/pod", server.getPodHandler)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
