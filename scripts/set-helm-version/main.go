package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/goccy/go-yaml"
)

func Run() error {
	if len(os.Args) < 3 {
		return fmt.Errorf("usage: set-helm-version [chart-path] [version]")
	}
	chartPath := os.Args[1]
	version := os.Args[2]

	file := filepath.Join(chartPath, "Chart.yaml")
	contents, err := os.ReadFile(file)
	if err != nil {
		return err
	}

	data := map[string]any{}
	err = yaml.Unmarshal(contents, &data)
	if err != nil {
		return err
	}

	data["version"] = version
	data["appVersion"] = version

	contents, err = yaml.Marshal(data)
	if err != nil {
		return err
	}

	err = os.WriteFile(file, contents, 0644)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	err := Run()

	code := 0
	if err != nil {
		fmt.Printf("error: %v", err)
		code = 1
	}
	os.Exit(code)
}
