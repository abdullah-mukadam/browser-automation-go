package main

import (
	"github.com/go-rod/rod"
	"github.com/go-rod/rod/lib/launcher"
	"time"
	"fmt"
)

func main() {
	// 1. Setup Browser
	url := launcher.New().Headless(false).MustLaunch()
	browser := rod.New().ControlURL(url).MustConnect()
	defer browser.MustClose()
	page := browser.MustPage()

	// 3. Execute Graph
	// --- Step 1 ---
	page.MustNavigate("https://www.google.com/?zx=1767185133577&no_sw_cr=1").MustWaitLoad()
	// --- Step 2 ---
	page.MustNavigate("https://www.google.com/?zx=1767185133577&no_sw_cr=1").MustWaitLoad()
	// --- Step 3 ---
	page.MustElement("body > div:nth-child(3) > div:nth-child(4) > div:nth-child(3)").MustWaitVisible().MustClick()
	// --- Step 4 ---
	page.MustNavigate("https://www.google.com/?zx=1767185133577&no_sw_cr=1").MustWaitLoad()
	// --- Step 5 ---
	page.MustNavigate("https://www.google.com/?zx=1767185133577&no_sw_cr=1").MustWaitLoad()
	// --- Step 6 ---
	page.MustNavigate("https://www.wikipedia.org/").MustWaitLoad()
	// --- Step 7 ---
	page.MustNavigate("https://www.wikipedia.org/").MustWaitLoad()
	// --- Step 8 ---
	page.MustNavigate("https://www.wikipedia.org/").MustWaitLoad()
	// --- Step 9 ---
	page.MustElement("body").MustWaitVisible().MustClick()
	// --- Step 10 ---
	page.MustElement("#searchInput").MustWaitVisible().MustClick()
	// --- Step 11 ---
	page.MustElement("span.lang-list-button-text.jsl10n").MustWaitVisible().MustClick()

	fmt.Println("✅ Automation Complete")
	time.Sleep(2 * time.Second)
}
