package main

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"os"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var zapLogger, _ = zap.NewProduction()

func main() {
	log := zapLogger.Sugar()

	// Init variables
	// Hard-coded key will be fetched from elsewhere
	vaultPodName := "vault"
	vaultUnsealKey := "3J2+sl2WNO625wDLhQbjnXj0s3qqYS39BVcuqnmweKyf"
	apiServer := os.Getenv("KUBERNETES_APISERVER")

	// Get the serviceaccount token
	bToken, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/token")
	if err != nil {
		log.Fatalf("Error reading file: %s", err)
	}
	token := string(bToken)

	// Create a new Kubernetes clientset using the serviceaccount token
	config := &rest.Config{
		Host:        apiServer,
		BearerToken: token,
		TLSClientConfig: rest.TLSClientConfig{
			Insecure: true,
		},
	}
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		log.Fatal(err)
	}

	// Get the namespace where this container is running from
	bNamespace, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	if err != nil {
		log.Fatalf("Error reading file: %s", err)
	}
	namespace := string(bNamespace)

	// Get all endpoints for vault
	endpoint, err := clientset.CoreV1().Endpoints(namespace).Get(context.Background(), vaultPodName, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	// Get the addresses associated to the endpoints
	for _, address := range endpoint.Subsets[0].Addresses {
		// For each vault instance, attempt to unseal with the key we have
		var jsonStr = []byte(fmt.Sprintf(`{"key": "%s"}`, vaultUnsealKey))
		req, err := http.NewRequest("POST", fmt.Sprintf("http://%s:8200/v1/sys/unseal", address.IP), bytes.NewBuffer(jsonStr))
		if err != nil {
			log.Fatalf("Error unsealing: %s", err)
		}
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			log.Fatalf("Error unsealing: %s", err)
		}
		defer resp.Body.Close()

		// Print the body to console for now
		println(resp.Body)
	}
}
