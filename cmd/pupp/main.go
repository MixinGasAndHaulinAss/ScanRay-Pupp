package main

import (
	"log"

	"github.com/NCLGISA/ScanRay-Pupp/internal/agent"
	"github.com/NCLGISA/ScanRay-Pupp/internal/config"
)

func main() {
	log.Println("=== ScanRay Pupp Agent ===")
	log.Println("Remote scanning agent for ScanRay Console")

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Configuration error: %v", err)
	}

	log.Printf("Pupp ID: %s", cfg.PuppID)
	log.Printf("Console: %s", cfg.ConsoleURL)

	a := agent.New(cfg)
	a.Run()
}
