package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"

	"github.com/PuerkitoBio/goquery"
)

const MAX_PAGE = 508
const BASE_URL = "https://go.drugbank.com/drugs?approved=0&nutraceutical=0&illicit=0&investigational=0&withdrawn=0&experimental=0&us=0&ca=0&eu=0&commit=Apply+Filter&page="

type DrugLink struct {
	Name string `json:"name,omitempty"`
	Link string `json:"link,omitempty"`
}

type Weights struct {
	Average      MolWeight `json:"average"`
	Monoisotopic MolWeight `json:"monoisotopic"`
}

type MolWeight struct {
	Type   string  `json:"type,omitempty"`
	Weight float64 `json:"weight,omitempty"`
	Units  string  `json:"units,omitempty"`
}

type DrugInfo struct {
	Smiles           string              `json:"smiles,omitempty"`
	ID               string              `json:"id,omitempty"`
	Molecule         string              `json:"molecule,omitempty"`
	IupacName        string              `json:"iupac_name,omitempty"`
	Summary          string              `json:"summary,omitempty"`
	Weight           []MolWeight         `json:"weight,omitempty"`
	Formula          string              `json:"formula,omitempty"`
	Description      string              `json:"description,omitempty"`
	Categories       []string            `json:"categories,omitempty"`
	Link             string              `json:"link,omitempty"`
	Type             string              `json:"type,omitempty"`
	Groups           string              `json:"groups,omitempty"`
	Synonyms         []string            `json:"synonyms,omitempty"`
	Indication       string              `json:"indication,omitempty"`
	Pharmacodynamics string              `json:"pharmacodynamics,omitempty"`
	Moa              []map[string]string `json:"moa,omitempty"`
}

// PrettyPrint takes any input and attempts to pretty print it
func PrettyPrint(data interface{}) {
	// Use reflection to check if the input is a pointer
	val := reflect.ValueOf(data)
	if val.Kind() == reflect.Ptr && !val.IsNil() {
		val = val.Elem()
	}

	// Convert non-nil pointers or values to JSON
	if val.IsValid() {
		jsonData, err := json.MarshalIndent(val.Interface(), "", "    ")
		if err != nil {
			log.Fatalf("Error marshaling data: %s", err)
		}
		fmt.Println(string(jsonData))
	} else {
		fmt.Println("nil")
	}
}

func fetch(url string) ([]byte, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("error fetching URL: %v", err)
	}
	defer resp.Body.Close()

	// Read the response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("error reading body: %v", err)
	}
	return body, nil
}

func getPage(url string) (*goquery.Document, error) {
	body, err := fetch(url)
	if err != nil {
		return nil, err
	}

	// Parse the HTML
	page, err := goquery.NewDocumentFromReader(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("error parsing HTML: %v", err)
	}

	return page, nil
}

// toSentenceCase capitalizes the first letter of every sentence in the string.
func toSentenceCase(input string) string {
	var result strings.Builder
	capitalizeNext := true

	for _, runeValue := range input {
		if capitalizeNext && unicode.IsLetter(runeValue) {
			runeValue = unicode.ToUpper(runeValue)
			capitalizeNext = false
		} else if runeValue == '.' || runeValue == '!' || runeValue == '?' {
			capitalizeNext = true
		}

		result.WriteRune(runeValue)
	}

	return result.String()
}

func getIntFromUserInput(s string) int {
	var input int
	fmt.Printf("⭐ %s: ", s)
	_, err := fmt.Scanln(&input)
	if err != nil {
		fmt.Println("Failed to read input:", err)
		return 0
	}
	return input
}

func getLinksPerPage(page *goquery.Document) []DrugLink {
	var links []DrugLink
	page.Find("#drugs-table tr").Each(func(_ int, s *goquery.Selection) {
		cols := s.Find("td")
		if cols.Length() > 0 {
			href, hasLink := cols.Eq(0).Find("a").Attr("href")
			link := ""
			if hasLink {
				link = fmt.Sprintf("https://go.drugbank.com%s", href)
			}

			// Extract drug information from columns
			drugInfo := DrugLink{
				Name: cols.Eq(0).Text(),
				Link: link,
			}

			// Append the drugInfo to the drugs slice
			links = append(links, drugInfo)
		}
	})
	return links
}

func normalize(s string) string {
	return strings.TrimSpace(strings.ToLower(s))
}

// Define a type for the handler functions
type fieldHandler func(*goquery.Selection, *DrugInfo)

// Create handler functions
func handleSynonyms(sibling *goquery.Selection, json *DrugInfo) {
	if sibling == nil || json == nil {
		return // Add appropriate handling or logging
	}
	sibling.Find("li").Each(func(_ int, li *goquery.Selection) {
		synonym := li.Text()
		json.Synonyms = append(json.Synonyms, synonym)
	})
}

func handleError(err error) {
	if err != nil {
		// Fire emoji error
		fmt.Println("🔥 Error: ", err)
		return
	}
}

func handleDescription(sibling *goquery.Selection, json *DrugInfo) {
	content := normalize(sibling.Text())
	if content == "not available" {
		content = "N/A"
	}
	json.Description = toSentenceCase(content)
}

func handleMechanismOfAction(sibling *goquery.Selection, json *DrugInfo) {
	if sibling == nil || json == nil {
		return // Add appropriate handling or logging
	}
	var moas []map[string]string
	sibling.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
		moa := make(map[string]string)
		tr.Find("td").Each(func(i int, td *goquery.Selection) {
			switch i {
			case 0:
				moa["receptor"] = td.Text()
			case 1:
				moa["mechanism"] = td.Text()
			case 2:
				moa["organism"] = td.Text()
			default:
				moa["unknown"] = td.Text()
			}
		})
		moas = append(moas, moa)
	})
	json.Moa = moas
}

func handleMolecularWeight(sibling *goquery.Selection, json *DrugInfo) {
	content := normalize(sibling.Text())
	fmt.Println("🔥 handleMolecularWeight()(content):", content)
	parts := strings.Split(content, " ")
	fmt.Println("🔥 partsLength:", len(parts))
	PrettyPrint(parts)

	if len(parts) == 4 {
		average, _ := strconv.ParseFloat(parts[1], 64)
		monoisotopic, _ := strconv.ParseFloat(parts[3], 64)

		json.Weight = append(json.Weight, MolWeight{
			Type:   "average",
			Weight: average,
			Units:  parts[2],
		})
		json.Weight = append(json.Weight, MolWeight{
			Type:   "monoisotopic",
			Weight: monoisotopic,
			Units:  parts[2],
		})
	} else {
		panic("🔥 handleMolecularWeight() failed to parse molecular weight")
	}
}

var fieldHandlers = map[string]fieldHandler{
	"synonyms":            handleSynonyms,
	"description":         handleDescription,
	"weight":              handleMolecularWeight,
	"mechanism of action": handleMechanismOfAction,
	// Add other handlers here...
}

func displayDrugDetails(drugInfo DrugInfo) {
	fmt.Print("-------------------------\n🧬 Molecule: ")
	fmt.Print(drugInfo.Molecule)
	fmt.Print("\n          L: ")
	fmt.Print(drugInfo.Link)
	fmt.Print("\n         ID: ")
	fmt.Print(drugInfo.ID)
	fmt.Print("\n          S: ")
	fmt.Print(drugInfo.Smiles)
	fmt.Println("\n-------------------------\n")
}

func main() {

	// get user input for page number
	count := getIntFromUserInput("Enter num pages to scrape")

	// check if count is within range
	if count > MAX_PAGE {
		fmt.Printf("❌ Max page is %v\n", MAX_PAGE)
		return
	}

	links := make([]DrugLink, 0)

	for i := 0; i < count; i++ {
		page, err := getPage(fmt.Sprintf(BASE_URL+"%d", i))
		if err != nil {
			log.Printf("Error getting page: %v\n", err)
			continue
		}
		links = append(links, getLinksPerPage(page)...)
		fmt.Printf("ℹ️ collected %v total links... \n", len(links))
	}

	var wg sync.WaitGroup
	drugInfosChan := make(chan DrugInfo, count) // Buffered channel to collect results

	for _, link := range links {
		wg.Add(1)

		go func(pageLink DrugLink) {
			defer wg.Done()

			page, err := getPage(pageLink.Link)
			if err != nil {
				panic(err)
			}

			json := DrugInfo{
				Link: pageLink.Link,
			}

			// Create a map that links the normalized title to the corresponding field in the struct
			scrapedFieldsMap := map[string]*string{
				"generic name":              &json.Molecule,
				"link":                      &json.Link,
				"drugbank accession number": &json.ID,
				"smiles":                    &json.Smiles,
				"iupac":                     &json.IupacName,
				"weight":                    nil,
				"chemical formula":          &json.Formula,
				"description":               &json.Description,
				"synonyms":                  nil,
				"mechanism of action":       nil,
			}

			page.Find("dl").Find("dt").Each(func(_ int, s *goquery.Selection) {
				title := normalize(s.Text())
				sibling := s.Next()

				if handler, exists := fieldHandlers[title]; exists {
					if sibling == nil {
						panic("🔥 sibling is nil")
					}
					handler(sibling, &json)
				} else if thisFieldPtr, exists := scrapedFieldsMap[title]; exists {
					content := normalize(sibling.Text())
					if content == "not available" {
						content = "N/A"
					}
					if thisFieldPtr == nil {
						panic("🔥 thisFieldPtr is nil, this likely means that the field is not yet handled in scrapedFieldsMap or fieldHandlers")
					}
					*thisFieldPtr = content
				}
			})

			drugInfosChan <- json
		}(link)
	}

	// Close the channel after all goroutines are done
	go func() {
		wg.Wait()
		close(drugInfosChan)
	}()

	// Process results from the channel
	for drugInfo := range drugInfosChan {
		displayDrugDetails(drugInfo)
		PrettyPrint(drugInfo)
	}

}
