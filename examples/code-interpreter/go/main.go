package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"time"

	liteboxd "github.com/fslongjin/liteboxd/sdk/go"
)

func main() {
	baseURL := os.Getenv("LITEBOXD_BASE_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080/api/v1"
	}
	template := os.Getenv("LITEBOXD_TEMPLATE")
	if template == "" {
		template = "code-interpreter"
	}
	timeout := 30 * time.Second

	client := liteboxd.NewClient(baseURL, liteboxd.WithTimeout(timeout))
	ctx := context.Background()

	sandbox, err := client.Sandbox.Create(ctx, template, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		_ = client.Sandbox.Delete(ctx, sandbox.ID)
	}()

	_, err = client.Sandbox.WaitForReady(ctx, sandbox.ID, 2*time.Second, 5*time.Minute)
	if err != nil {
		log.Fatal(err)
	}

	resp, err := client.Sandbox.Execute(ctx, sandbox.ID, []string{"python", "-c", "print('hello from liteboxd')"}, 30)
	if err != nil {
		log.Fatal(err)
	}
	payload, err := json.Marshal(resp)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(string(payload))
	if resp.Stderr != "" {
		fmt.Fprintln(os.Stderr, resp.Stderr)
	}
}
