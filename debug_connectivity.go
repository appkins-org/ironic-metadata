package main

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os/exec"
	"time"
)

func main() {
	url := "https://ironic.appkins.io/v1/"

	fmt.Printf("=== Testing connectivity to %s ===\n", url)

	// Test 1: Basic curl
	fmt.Println("\n1. Testing with curl...")
	cmd := exec.Command("curl", "-I", url)
	output, err := cmd.Output()
	if err != nil {
		fmt.Printf("Curl failed: %v\n", err)
	} else {
		fmt.Printf("Curl succeeded: %s\n", string(output)[:100])
	}

	// Test 2: Go HTTP client (default)
	fmt.Println("\n2. Testing with default Go HTTP client...")
	client1 := &http.Client{Timeout: 10 * time.Second}
	resp, err := client1.Get(url)
	if err != nil {
		fmt.Printf("Default client failed: %v\n", err)
	} else {
		resp.Body.Close()
		fmt.Printf("Default client succeeded: %d\n", resp.StatusCode)
	}

	// Test 3: Go HTTP client with custom transport
	fmt.Println("\n3. Testing with custom transport...")
	client2 := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			Dial: (&net.Dialer{
				Timeout: 5 * time.Second,
			}).Dial,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: false},
		},
	}
	resp, err = client2.Get(url)
	if err != nil {
		fmt.Printf("Custom transport failed: %v\n", err)
	} else {
		resp.Body.Close()
		fmt.Printf("Custom transport succeeded: %d\n", resp.StatusCode)
	}

	// Test 4: Direct TCP connection
	fmt.Println("\n4. Testing direct TCP connection...")
	conn, err := net.DialTimeout("tcp", "10.0.60.10:443", 5*time.Second)
	if err != nil {
		fmt.Printf("Direct TCP connection failed: %v\n", err)
	} else {
		conn.Close()
		fmt.Printf("Direct TCP connection succeeded\n")
	}
}
