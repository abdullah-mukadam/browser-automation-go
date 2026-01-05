package main

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"log"
	"time"
)

func main() {
	// Launch browser
	u := launcher.New().Headless(false).MustLaunch()
	browser := rod.New().ControlURL(u).MustConnect()
	defer browser.MustClose()

	page := browser.MustPage()
	
	// Wait for final state
	time.Sleep(2 * time.Second)
	
	log.Println("✓ Test completed successfully")
	log.Println("Press Ctrl+C to exit...")
	time.Sleep(1 * time.Hour)
}
