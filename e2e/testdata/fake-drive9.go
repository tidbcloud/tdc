package main

import (
	"encoding/json"
	"fmt"
	"os"
)

type call struct {
	Args       []string `json:"args"`
	Home       string   `json:"home"`
	APIKey     string   `json:"api_key"`
	Server     string   `json:"server"`
	RegionCode string   `json:"region_code"`
}

func main() {
	record := os.Getenv("FAKE_DRIVE9_RECORD")
	if record != "" {
		file, err := os.OpenFile(record, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		if err != nil {
			panic(err)
		}
		_ = json.NewEncoder(file).Encode(call{
			Args:       os.Args[1:],
			Home:       os.Getenv("HOME"),
			APIKey:     os.Getenv("DRIVE9_API_KEY"),
			Server:     os.Getenv("DRIVE9_SERVER"),
			RegionCode: os.Getenv("DRIVE9_REGION_CODE"),
		})
		_ = file.Close()
	}
	args := os.Args[1:]
	if len(args) >= 1 && args[0] == "create" {
		name := flagValue(args, "--name")
		_ = json.NewEncoder(os.Stdout).Encode(map[string]string{
			"tenant_id":      "tenant-" + name,
			"api_key":        "key-" + name,
			"status":         "provisioned",
			"cloud_provider": "aws",
			"region_code":    os.Getenv("DRIVE9_REGION_CODE"),
		})
		return
	}
	if len(args) >= 2 && args[0] == "vault" && args[1] == "ls" {
		fmt.Println(`{"secrets":[]}`)
		return
	}
	if len(args) >= 2 && args[0] == "journal" && args[1] == "new" {
		fmt.Println(`{}`)
		return
	}
	if len(args) >= 2 && args[0] == "fs" && args[1] == "stat" {
		fmt.Println(`{"path":"/","size":0,"isdir":true}`)
	}
}

func flagValue(args []string, name string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == name {
			return args[i+1]
		}
	}
	return ""
}
