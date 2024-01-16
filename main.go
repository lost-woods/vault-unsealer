package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	zapLogger, _ = zap.NewProduction()
	log          = zapLogger.Sugar()

	apiServer     = os.Getenv("KUBERNETES_APISERVER")
	vaultName     = os.Getenv("VAULT_ENDPOINT_NAME")
	vaultPort     = getEnvInt("VAULT_PORT")
	secondsToWait = getEnvInt("REFRESH_TIME")
)

type VaultStatus struct {
	Type           string `json:"type"`
	Initialized    bool   `json:"initialized"`
	Sealed         bool   `json:"sealed"`
	KeysThreshold  int    `json:"t"`
	KeysIssued     int    `json:"n"`
	UnsealProgress int    `json:"progress"`
	Nonce          string `json:"nonce"`
	Version        string `json:"version"`
	BuildDate      string `json:"build_date"`
	Migration      bool   `json:"migration"`
	RecoverySeal   bool   `json:"recovery_seal"`
	StorageType    string `json:"storage_type"`
}

func getEnvInt(key string) int {
	envStr := os.Getenv(key)

	env, err := strconv.Atoi(envStr)
	if err != nil {
		log.Fatalf("Error parsing env variable %s: %s", key, err)
		return 0
	}

	return env
}

func readFile(path string) string {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		log.Fatalf("Error reading file: %s", err)
		return ""
	}

	return strings.TrimSuffix(string(fileBytes), "\n")
}

func sendRequest(method string, ip string, endpoint string, body io.Reader) ([]byte, error) {
	req, err := http.NewRequest(method, fmt.Sprintf("http://%s:%d/%s", ip, vaultPort, endpoint), body)
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
	return out, nil
}

func main() {
	// Variables
	vaultUnsealKey := readFile("/var/run/vault/key")
	namespace := readFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	token := readFile("/var/run/secrets/kubernetes.io/serviceaccount/token")

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

	// Main control loop
	for {
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

			var status VaultStatus
			err = json.Unmarshal(body, &status)
			if err != nil {
				log.Fatalf("Error parsing response body: %s", err)
			}

			if status.Sealed {
				// Attempt to unseal with the key we have
				var jsonStr = []byte(fmt.Sprintf(`{"key": "%s"}`, vaultUnsealKey))
				_, err = sendRequest("POST", address.IP, "v1/sys/unseal", bytes.NewBuffer(jsonStr))
				if err != nil {
					log.Fatalf("Error unsealing: %s", err)
				}

				log.Infof("Sent unseal request to instance at IP %s.", address.IP)
			} else {
				log.Infof("Vault instance at IP %s is already unsealed.", address.IP)
			}
		}

		// Print a success message
		log.Info("Vault unseal actions complete.")
		time.Sleep(time.Duration(secondsToWait) * time.Second)
	}
}
