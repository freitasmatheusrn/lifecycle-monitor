package products

import (
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/playwright-community/playwright-go"
)

type Crawler struct {
	pw         *playwright.Playwright
	browser    playwright.Browser
	baseURL    string
	mu         sync.Mutex
	isRunning  bool
}

func NewCrawler(baseURL string) (*Crawler, error) {
	return &Crawler{
		baseURL: baseURL,
	}, nil
}

func (c *Crawler) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		return nil
	}

	pw, err := playwright.Run()
	if err != nil {
		return fmt.Errorf("could not start playwright: %w", err)
	}
	c.pw = pw

	browser, err := pw.Chromium.Launch(playwright.BrowserTypeLaunchOptions{
		Headless: playwright.Bool(true),
	})
	if err != nil {
		c.pw.Stop()
		return fmt.Errorf("could not launch browser: %w", err)
	}
	c.browser = browser
	c.isRunning = true

	return nil
}

func (c *Crawler) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		return nil
	}

	if c.browser != nil {
		if err := c.browser.Close(); err != nil {
			return fmt.Errorf("could not close browser: %w", err)
		}
	}

	if c.pw != nil {
		if err := c.pw.Stop(); err != nil {
			return fmt.Errorf("could not stop playwright: %w", err)
		}
	}

	c.isRunning = false
	return nil
}

func (c *Crawler) Collect(productCode string) (*CrawledData, error) {
	c.mu.Lock()
	if !c.isRunning {
		c.mu.Unlock()
		return nil, fmt.Errorf("crawler is not running")
	}
	c.mu.Unlock()

	page, err := c.browser.NewPage()
	if err != nil {
		return nil, fmt.Errorf("could not create page: %w", err)
	}
	defer page.Close()

	url := fmt.Sprintf("%s/%s", c.baseURL, productCode)

	if _, err := page.Goto(url, playwright.PageGotoOptions{
		WaitUntil: playwright.WaitUntilStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("could not navigate to %s: %w", url, err)
	}

	// Wait for product content to load
	if err := page.WaitForLoadState(playwright.PageWaitForLoadStateOptions{
		State: playwright.LoadStateNetworkidle,
	}); err != nil {
		return nil, fmt.Errorf("timeout waiting for page load: %w", err)
	}

	// ===== DEBUG START =====
	fmt.Printf("DEBUG: Requested URL: %s\n", url)
	fmt.Printf("DEBUG: Final URL after navigation: %s\n", page.URL())

	// Save screenshot to see what the page looks like
	screenshotPath := fmt.Sprintf("/tmp/debug_%s.png", productCode)
	if _, err := page.Screenshot(playwright.PageScreenshotOptions{
		Path:     playwright.String(screenshotPath),
		FullPage: playwright.Bool(true),
	}); err != nil {
		fmt.Printf("DEBUG: Failed to save screenshot: %v\n", err)
	} else {
		fmt.Printf("DEBUG: Screenshot saved to %s\n", screenshotPath)
	}

	// Save HTML to inspect the actual structure
	debugHTML, _ := page.Content()
	htmlPath := fmt.Sprintf("/tmp/debug_%s.html", productCode)
	if err := os.WriteFile(htmlPath, []byte(debugHTML), 0644); err != nil {
		fmt.Printf("DEBUG: Failed to save HTML: %v\n", err)
	} else {
		fmt.Printf("DEBUG: HTML saved to %s (length: %d)\n", htmlPath, len(debugHTML))
	}

	// Check if main selectors exist
	count1, _ := page.Locator("p.intro-section__content-headline-details--alternative").Count()
	count2, _ := page.Locator(".intro-section__content-headline-details p").Count()
	fmt.Printf("DEBUG: Selector 1 (p.intro-section__content-headline-details--alternative) count: %d\n", count1)
	fmt.Printf("DEBUG: Selector 2 (.intro-section__content-headline-details p) count: %d\n", count2)

	// Try broader selectors to understand page structure
	introSection, _ := page.Locator(".intro-section").Count()
	allIntroP, _ := page.Locator(".intro-section p").Count()
	fmt.Printf("DEBUG: .intro-section elements: %d\n", introSection)
	fmt.Printf("DEBUG: .intro-section p elements: %d\n", allIntroP)
	// ===== DEBUG END =====

	// Extract product data using selectors
	data := &CrawledData{}

	// Extract product description from the intro section headline
	// Wait for the element to be visible (Angular apps need time to render)
	descElement := page.Locator("p.intro-section__content-headline-details--alternative")
	if err := descElement.First().WaitFor(playwright.LocatorWaitForOptions{
		State:   playwright.WaitForSelectorStateVisible,
		Timeout: playwright.Float(10000),
	}); err == nil {
		desc, err := descElement.First().TextContent()
		if err == nil {
			data.Description = strings.TrimSpace(desc)
		}
	}

	// Fallback: try alternative selector if main one didn't work
	if data.Description == "" {
		// Try getting from the headline details div
		altElement := page.Locator(".intro-section__content-headline-details p")
		if err := altElement.First().WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(5000),
		}); err == nil {
			desc, err := altElement.First().TextContent()
			if err == nil {
				data.Description = strings.TrimSpace(desc)
			}
		}
	}

	// Extract product lifecycle status
	// The structure is: div.product-metadata-item containing p.product-metadata-item__label with "Product lifecycle"
	// and the value is in the sibling div.product-metadata-item__label-wrapper > p
	lifecycleItems := page.Locator("div.product-metadata-item")
	if count, _ := lifecycleItems.Count(); count > 0 {
		for i := 0; i < count; i++ {
			item := lifecycleItems.Nth(i)
			labelElement := item.Locator("p.product-metadata-item__label")
			if labelCount, _ := labelElement.Count(); labelCount > 0 {
				label, err := labelElement.First().TextContent()
				if err == nil && strings.Contains(strings.ToLower(label), "product lifecycle") {
					// Found the lifecycle item, now get the value
					valueElement := item.Locator("div.product-metadata-item__label-wrapper p")
					if valueCount, _ := valueElement.Count(); valueCount > 0 {
						value, err := valueElement.First().TextContent()
						if err == nil {
							// Clean the text (remove icon text and extra whitespace)
							data.Status = cleanLifecycleStatus(value)
							break
						}
					}
				}
			}
		}
	}

	// Extract replacement product code if status indicates product is being discontinued
	// Status values that have successor: "Prod. Cancellation", "End Prod.Lifecycl.", "Prod. Discont."
	fmt.Printf("DEBUG [ReplacementCode]: Status extracted = '%s'\n", data.Status)
	fmt.Printf("DEBUG [ReplacementCode]: Status trimmed = '%s'\n", strings.TrimSpace(data.Status))
	fmt.Printf("DEBUG [ReplacementCode]: Status bytes = %v\n", []byte(strings.TrimSpace(data.Status)))

	statusTrimmed := strings.TrimSpace(data.Status)
	conditionProdCancellation := statusTrimmed == "Prod. Cancellation"
	conditionEndLifecycle := statusTrimmed == "End Prod.Lifecycl."
	conditionProdDiscont := statusTrimmed == "Prod. Discont."
	fmt.Printf("DEBUG [ReplacementCode]: Condition (Prod. Cancellation): %v\n", conditionProdCancellation)
	fmt.Printf("DEBUG [ReplacementCode]: Condition (End Prod.Lifecycl.): %v\n", conditionEndLifecycle)
	fmt.Printf("DEBUG [ReplacementCode]: Condition (Prod. Discont.): %v\n", conditionProdDiscont)

	if conditionProdCancellation || conditionEndLifecycle || conditionProdDiscont {
		fmt.Printf("DEBUG [ReplacementCode]: Entered replacement code extraction block\n")

		// Check how many elements each selector finds
		successorCount, _ := page.Locator("sie-ui-richtext .primary-label").Count()
		fmt.Printf("DEBUG [ReplacementCode]: 'sie-ui-richtext .primary-label' count: %d\n", successorCount)

		// Try alternative selectors to understand the structure
		alt1, _ := page.Locator("sie-ui-richtext").Count()
		alt2, _ := page.Locator(".primary-label").Count()
		alt3, _ := page.Locator("sie-ui-link").Count()
		alt4, _ := page.Locator("sie-ui-link .primary-label").Count()
		alt5, _ := page.Locator("[class*='successor']").Count()
		alt6, _ := page.Locator("[class*='replacement']").Count()
		fmt.Printf("DEBUG [ReplacementCode]: 'sie-ui-richtext' count: %d\n", alt1)
		fmt.Printf("DEBUG [ReplacementCode]: '.primary-label' count: %d\n", alt2)
		fmt.Printf("DEBUG [ReplacementCode]: 'sie-ui-link' count: %d\n", alt3)
		fmt.Printf("DEBUG [ReplacementCode]: 'sie-ui-link .primary-label' count: %d\n", alt4)
		fmt.Printf("DEBUG [ReplacementCode]: '[class*=successor]' count: %d\n", alt5)
		fmt.Printf("DEBUG [ReplacementCode]: '[class*=replacement]' count: %d\n", alt6)

		// Look for the successor link in the richtext element
		// The structure is: sie-ui-richtext containing sie-ui-link with a .primary-label div
		successorElement := page.Locator("sie-ui-richtext .primary-label")
		if err := successorElement.First().WaitFor(playwright.LocatorWaitForOptions{
			State:   playwright.WaitForSelectorStateVisible,
			Timeout: playwright.Float(5000),
		}); err == nil {
			fmt.Printf("DEBUG [ReplacementCode]: WaitFor succeeded, element is visible\n")
			// Try to get the title attribute first (more reliable)
			if title, err := successorElement.First().GetAttribute("title"); err == nil && title != "" {
				fmt.Printf("DEBUG [ReplacementCode]: Got title attribute = '%s'\n", title)
				data.ReplacementCode = strings.TrimSpace(title)
			} else {
				fmt.Printf("DEBUG [ReplacementCode]: Title attribute empty or error, trying TextContent\n")
				// Fallback to text content
				if text, err := successorElement.First().TextContent(); err == nil {
					fmt.Printf("DEBUG [ReplacementCode]: Got text content = '%s'\n", text)
					data.ReplacementCode = strings.TrimSpace(text)
				} else {
					fmt.Printf("DEBUG [ReplacementCode]: TextContent error: %v\n", err)
				}
			}
		} else {
			fmt.Printf("DEBUG [ReplacementCode]: WaitFor FAILED: %v\n", err)

			// Try to get any text from sie-ui-richtext even if not visible
			richtextElement := page.Locator("sie-ui-richtext")
			if richtextCount, _ := richtextElement.Count(); richtextCount > 0 {
				for i := 0; i < richtextCount; i++ {
					text, _ := richtextElement.Nth(i).TextContent()
					html, _ := richtextElement.Nth(i).InnerHTML()
					fmt.Printf("DEBUG [ReplacementCode]: sie-ui-richtext[%d] text = '%s'\n", i, text)
					fmt.Printf("DEBUG [ReplacementCode]: sie-ui-richtext[%d] innerHTML = '%s'\n", i, html)
				}
			}
		}
	} else {
		fmt.Printf("DEBUG [ReplacementCode]: Status does NOT match cancellation conditions, skipping replacement extraction\n")
	}

	fmt.Printf("DEBUG [ReplacementCode]: Final ReplacementCode = '%s'\n", data.ReplacementCode)

	// Get raw HTML for debugging/archival
	html, err := page.Content()
	if err == nil {
		data.RawHTML = html
	}

	// Validate we got at least the description
	if data.Description == "" {
		return nil, fmt.Errorf("could not extract product description for code %s", productCode)
	}

	return data, nil
}

func cleanLifecycleStatus(status string) string {
	status = strings.TrimSpace(status)
	// The status text may contain extra content from icons or tooltips
	// Valid values: "Active Product", "Phase Out Announce", "Prod. Cancellation", "End Prod.Lifecycl.", "Prod. Discont."
	// Take only the first meaningful part before any icon content
	lines := strings.Split(status, "\n")
	if len(lines) > 0 {
		status = strings.TrimSpace(lines[0])
	}

	// Known status values to match against
	knownStatuses := []string{
		"Active Product",
		"Phase Out Announce",
		"Prod. Cancellation",
		"End Prod.Lifecycl.",
		"Prod. Discont.",
	}

	// Try to match against known statuses
	statusLower := strings.ToLower(status)
	for _, known := range knownStatuses {
		if strings.Contains(statusLower, strings.ToLower(known)) {
			return known
		}
	}

	// Fallback: return cleaned up status
	fields := strings.Fields(status)
	if len(fields) >= 2 {
		return strings.Join(fields[:2], " ")
	}
	if len(fields) == 1 {
		return fields[0]
	}
	return status
}
