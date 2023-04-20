package main

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	zapLogger, _ = zap.NewProduction()
	log          = zapLogger.Sugar()

	apiServer = os.Getenv("KUBERNETES_APISERVER")
	vaultName = "vault"
	vaultPort = "8200"
)

func sendRequest(method string, ip string, endpoint string, body io.Reader) (string, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("http://%s:%s/%s", ip, vaultPort, endpoint), body)
	if err != nil {
		log.Fatalf("Error preparing request: %s", err)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Fatalf("Error performing request: %s", err)
	}

	// Return response body as string
	out, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Fatalf("Error reading response body: %s", err)
	}

	defer resp.Body.Close()
	return string(out), nil
}

func main() {
	// Init variables
	// Hard-coded key will be fetched from elsewhere
	vaultUnsealKey := "3J2+sl2WNO625wDLhQbjnXj0s3qqYS39BVcuqnmweKyf"

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
	endpoint, err := clientset.CoreV1().Endpoints(namespace).Get(context.Background(), vaultName, metav1.GetOptions{})
	if err != nil {
		log.Fatal(err)
	}

	// Get the addresses associated to the endpoints
	for _, address := range endpoint.Subsets[0].Addresses {
		// For each vault instance, check if vault is sealed
		body, err := sendRequest("GET", address.IP, "v1/sys/seal-status", nil)
		if err != nil {
			log.Fatalf("Error fetching seal status: %s", err)
		}

		log.Infof("Response: %s", body)

		// Attempt to unseal with the key we have
		var jsonStr = []byte(fmt.Sprintf(`{"key": "%s"}`, vaultUnsealKey))
		_, err = sendRequest("POST", address.IP, "v1/sys/unseal", bytes.NewBuffer(jsonStr))
		if err != nil {
			log.Fatalf("Error unsealing: %s", err)
		}

		// Print a success message
		log.Infof("Sent unseal request to instance at IP %s.", address.IP)
	}

	// Print a success message
	log.Info("Successfully sent unseal key to all vault instaces.")
}
