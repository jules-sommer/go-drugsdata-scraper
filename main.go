package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math/rand"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/PuerkitoBio/goquery"
	"github.com/briandowns/spinner"
)

const (
	MAX_PAGE                = 508
	BASE_URL                = "https://go.drugbank.com/drugs?approved=0&nutraceutical=0&illicit=0&investigational=0&withdrawn=0&experimental=0&us=0&ca=0&eu=0&commit=Apply+Filter&page="
	STUB_NOTICE_TEXT        = "this drug entry is a stub and has not been fully annotated. it is scheduled to be annotated soon."
	DRUG_INTERACTIONS_URL   = "https://go.drugbank.com/drugs/%s/drug_interactions.json?start=%d&length=%d&_=%d"
	DRUG_BASE_URL           = "https://go.drugbank.com/drugs/%s/"
	STRUCTURE_SVG_URL       = "https://go.drugbank.com/structures/%s/image.svg"
	STRUCTURE_THUMB_SVG_URL = "https://go.drugbank.com/structures/%s/thumb.svg"
)

var (
	stats_num_sleeps             int           = 0
	stats_duration_slept         time.Duration = 0
	stats_ratelimit_failures     []string      = make([]string, 0)
	stats_num_ratelimit_failures int           = 0
	stats_error_log              []string      = make([]string, 0)
	stats_num_errors             int           = 0
	stats_num_requests           int           = 0
	stats_num_retry              int           = 0
)

const (
	RetryLimit           = 4
	DelayBetweenRequests = 8 * time.Second  // Adjust as needed
	DelayAfterError      = 20 * time.Second // Delay after an error
)

func getFieldInfo(drugInfo *DrugInfo, fieldName string, opts ...string) string {

	res := htmlRawToFieldName(fieldName)
	// print debug
	return res
}

// Helper function: Checks if a string is contained in a slice
func stringSliceContains(slice []string, checkAll bool, values ...string) bool {
	for _, value := range values {
		found := false
		for _, item := range slice {
			if item == value {
				found = true
				break
			}
		}
		if checkAll && !found {
			return false
		} else if !checkAll && found {
			return true
		}
	}
	return checkAll
}

type (
	DrugLink struct {
		Name string `json:"name,omitempty"`
		Link string `json:"link,omitempty"`
	}

	Weights struct {
		Average      MolWeight `json:"average"`
		Monoisotopic MolWeight `json:"monoisotopic"`
	}

	InChiData struct {
		Hash string `json:"hash,omitempty"`
		ID   string `json:"id,omitempty"`
	}

	MolWeight struct {
		Type   string  `json:"type,omitempty"`
		Weight float64 `json:"weight,omitempty"`
		Units  string  `json:"units,omitempty"`
	}

	DrugInfo struct {
		Smiles               string              `json:"smiles,omitempty"`
		ID                   string              `json:"id,omitempty"`
		Molecule             string              `json:"molecule,omitempty"`
		CAS                  string              `json:"cas,omitempty"`
		IupacName            string              `json:"iupac_name,omitempty"`
		Background           string              `json:"background,omitempty"`
		InChI                InChiData           `json:"inchi,omitempty"`
		InChIKey             string              `json:"inchi_key,omitempty"`
		Summary              string              `json:"summary,omitempty"`
		Weight               []MolWeight         `json:"weight,omitempty"`
		Formula              string              `json:"formula,omitempty"`
		Description          string              `json:"description,omitempty"`
		Categories           []string            `json:"categories,omitempty"`
		Link                 string              `json:"link,omitempty"`
		Type                 string              `json:"type,omitempty"`
		Groups               []string            `json:"groups,omitempty"`
		Synonyms             []string            `json:"synonyms,omitempty"`
		Indication           string              `json:"indication,omitempty"`
		IsStub               bool                `json:"is_stub,omitempty"`
		Pharmacodynamics     string              `json:"pharmacodynamics,omitempty"`
		Moa                  []map[string]string `json:"moa,omitempty"`
		AdverseEffects       string              `json:"adverse_effects,omitempty"`
		DrugInteractions     [][]string          `json:"drug_interactions,omitempty"`
		DrugInteractionsPage []string            `json:"drug_interactions_page,omitempty"`
		HalfLife             string              `json:"half_life,omitempty"`
		RouteOfElimination   string              `json:"route_of_elimination,omitempty"`
		Toxicity             string              `json:"toxicity,omitempty"`
		Clearance            string              `json:"clearance,omitempty"`
		Absorption           string              `json:"absorption,omitempty"`
	}
)

func ExtractFieldsOfType[T any](input interface{}, inverse ...bool) map[string]*T {
	result := make(map[string]*T)
	val := reflect.ValueOf(input)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		panic("input must be a non-nil pointer to a struct")
	}
	structVal := val.Elem()
	if structVal.Kind() != reflect.Struct {
		panic("input must be a pointer to a struct")
	}

	targetType := reflect.TypeOf((*T)(nil)).Elem()

	inverseMode := false
	if len(inverse) > 0 {
		inverseMode = inverse[0]
	}

	for i := 0; i < structVal.NumField(); i++ {
		field := structVal.Field(i)
		name := structVal.Type().Field(i).Name

		if (inverseMode && field.Type() != targetType) || (!inverseMode && field.Type() == targetType) {
			if field.CanAddr() {
				fieldPtr := field.Addr()
				if fieldPtr.Type() == reflect.PtrTo(targetType) {
					result[name] = fieldPtr.Interface().(*T)
				} /* else if inverseMode {
				// 	// Handle inverse case where field type is not T
				// 	// This branch can be adjusted based on what you want to do with non-T fields
				*/
			}
		}
	}

	return result
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

func randTime(min, max time.Duration) time.Duration {
	return time.Duration(rand.Int63n(int64(max-min)) + int64(min))
}

func Sleep(duration time.Duration) {
	s := spinner.New(spinner.CharSets[34], 100*time.Millisecond)
	s.Color("fgCyan", "magenta", "bold")
	s.Start()
	stats_num_sleeps++
	time.Sleep(duration)
	s.Stop()
}

func fetchPage(url string, getDom ...bool) (string, *goquery.Document, error) {
	delay := randTime(0, DelayBetweenRequests)
	delayErr := randTime(DelayBetweenRequests, DelayAfterError)

	stats_num_requests++

	for i := 0; i < RetryLimit; i++ {
		resp, err := http.Get(url)
		if err != nil {
			logFetchError(err, fmt.Sprintf("Error fetching URL: %v, retrying...", url))
			clearTerminal()
			Sleep(delayErr)
			stats_num_retry++
			continue
		}
		defer resp.Body.Close()

		bodyBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			logFetchError(err, fmt.Sprintf("Error reading response body: %v, retrying...", url))
			Sleep(delayErr)
			stats_num_retry++
			continue
		}
		body := string(bodyBytes)
		if strings.Contains(body, "error code: 1015") {
			// terrible emoji disaster probably banned panic message
			logFetchError(err, fmt.Sprintf("💀🩸💀🩸💀🩸💀🩸💀 --- [!!DEATH IS UPON US, CLOUDFLARE BANNED!!] --- 💀🩸💀🩸💀🩸💀🩸💀\n%s", body))
			stats_num_retry++
			Sleep(delayErr)
			continue
		} else if strings.Contains(body, "page not found") {
			logFetchError(err, "💀🩸💀🩸💀🩸💀🩸💀 PAGE NOT FOUND 404 💀🩸💀🩸💀🩸💀🩸💀")
			stats_num_retry++
			Sleep(delayErr)
			continue
		}

		if len(getDom) > 0 && !getDom[0] {
			time.Sleep(delay) // Rate limit
			stats_num_sleeps++
			return body, nil, nil
		}

		doc, err := goquery.NewDocumentFromReader(strings.NewReader(body))

		doc.Find("a.track-link").Each(func(index int, item *goquery.Selection) {
			item.Remove()
		})

		if err != nil {
			logFetchError(err, fmt.Sprintf("Error parsing HTML: %v, retrying...", url))
			stats_num_retry++
			time.Sleep(delayErr)
			stats_num_sleeps++
			continue
		}

		time.Sleep(delay) // Rate limit
		stats_num_sleeps++
		return body, doc, nil
	}
	stats_num_ratelimit_failures++
	stats_ratelimit_failures = append(stats_ratelimit_failures, url)
	return "", nil, fmt.Errorf("failed to fetch page after %d retries", RetryLimit)
}

func logFetchError(err error, s ...string) {
	log.Println(s)
	_, file, line, _ := runtime.Caller(1)
	log.Printf("[FETCH_ERROR][%s][LINE: %d]: %v", file, line, err)

	logged := make([]string, len(s)+1)
	copy(logged[1:], s)

	logged[0] = fmt.Sprintf("[FETCH_ERROR][%s][LINE: %d]: %v", file, line, err)

	stats_error_log = append(stats_error_log, strings.Join(logged, "\n"))

	stats_num_errors++
}

//*func(): func getPageByNumRoutine():
// builds the []drugLink slice for a given page number and sends it to the linksChan
// allowing the main goroutine to continue processing the next page and populate the links slice concurrently.
// uses the above fetchPage function to get the page HTML and then uses the getLinksPerPage function to extract
// the drug links from the page.
/*
 * @param pageNum: the page number to scrape
 * @param wg: the WaitGroup to signal when the goroutine is done
 * @param linksChan: the channel to send the []DrugLink slice to
 ! @returns: void
*/
func getPageByNumRoutine(pageNum int, wg *sync.WaitGroup, linksChan chan<- []DrugLink) error {
	defer wg.Done()

	url := fmt.Sprintf(BASE_URL+"%d", pageNum)
	_, page, err := fetchPage(url)
	if err != nil {
		log.Printf("🔥 Error getting page: %v\n", err)
		return err
	}
	linksChan <- getLinksPerPage(page)
	return nil
}

func ToTitleCase(input string) string {
	var result strings.Builder
	capitalizeNext := true

	for _, runeValue := range input {
		if capitalizeNext && unicode.IsLetter(runeValue) {
			runeValue = unicode.ToUpper(runeValue)
			capitalizeNext = false
		} else if runeValue == ' ' || runeValue == '-' || runeValue == '\'' {
			capitalizeNext = true
		}

		result.WriteRune(runeValue)
	}

	return result.String()
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

func htmlRawToFieldName(htmlRaw string) string {

	htmlRaw = strings.ReplaceAll(strings.ReplaceAll(ToTitleCase(htmlRaw), " ", ""), "-", "")

	switch htmlRaw {
	case "DrugbankAccessionNumber":
		return "ID"
	case "GenericName":
		return "Molecule"
	case "CasNumber":
		return "CAS"
	}

	return htmlRaw
}

func clearTerminal() {
	cmd := exec.Command("clear")
	cmd.Stdout = os.Stdout
	cmd.Run()
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
type fieldHandler func(*goquery.Selection, interface{}, string) error

func handleDrugInteractions(sibling *goquery.Selection, fieldPtr interface{}, name string) error {
	json, ok := fieldPtr.(*DrugInfo)
	if !ok {
		return fmt.Errorf("handleDrugInteractions(): type assertion to *DrugInfo failed")
	}

	if sibling == nil || json == nil {
		return fmt.Errorf("handleDrugInteractions(): invalid arguments")
	}

	// Get the drug ID
	id := json.ID
	link := fmt.Sprintf(DRUG_INTERACTIONS_URL, id, 0, 100, time.Now().Unix())
	page, _, err := fetchPage(link)

	if err != nil {
		return fmt.Errorf("handleDrugInteractions(): failed to fetch page: %v", err)
	}

	entries := sibling.Find("#drug-interactions-table_info").Text()

	// Parse the JSON
	interactions, err := ParseDrugInteractions(page)

	if err != nil {
		return fmt.Errorf("handleDrugInteractions(): failed to parse JSON: %v", err)
	}

	// Assign the interactions to the DrugInfo
	json.DrugInteractions = interactions
	json.DrugInteractionsPage = append(make([]string, 1), entries, page)

	return nil

}

func handleListAsArray(sibling *goquery.Selection, fieldPtr interface{}, name string) error {
	if sibling == nil {
		return fmt.Errorf("handleListAsArray(): sibling is nil")
	}

	val := reflect.ValueOf(fieldPtr)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return fmt.Errorf("handleListAsArray(): fieldPtr must be a non-nil pointer")
	}

	// Ensure fieldPtr is a *[]string
	slicePtr, ok := fieldPtr.(*[]string)
	if !ok {
		return fmt.Errorf("handleListAsArray(): fieldPtr is not a *[]string")
	}

	sibling.Find("li").Each(func(_ int, li *goquery.Selection) {
		fieldsVal := strings.TrimSpace(li.Text())
		*slicePtr = append(*slicePtr, fieldsVal)
	})
	return nil
}

func handleDescription(sibling *goquery.Selection, fieldPtr interface{}, name string) error {
	description, ok := fieldPtr.(*string)
	if !ok {
		return fmt.Errorf("handleDescription: type assertion to *string failed")
	}

	if description == nil || sibling == nil {
		return fmt.Errorf("handleDescription: invalid arguments")
	}

	val := reflect.ValueOf(fieldPtr)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return fmt.Errorf("handleDescription: fieldPtr must be a non-nil pointer")
	}

	content := toSentenceCase(sibling.Text())
	*description = content

	return nil
}

func handleMechanismOfAction(sibling *goquery.Selection, fieldPtr interface{}, name string) error {
	json, ok := fieldPtr.(*DrugInfo)
	if !ok {
		return fmt.Errorf("handleMechanismOfAction: type assertion to *DrugInfo failed")
	}

	if sibling == nil || json == nil {
		return fmt.Errorf("handleMechanismOfAction: invalid arguments")
	}

	var moas []map[string]string

	sibling.Find("tbody tr").Each(func(_ int, tr *goquery.Selection) {
		moa := make(map[string]string)
		tr.Find("td").Each(func(i int, td *goquery.Selection) {
			switch i {
			case 0:
				moa["target"] = td.Text()
			case 1:
				moa["action"] = td.Text()
			case 2:
				moa["organism"] = td.Text()
			default:
				moa["unknown"] = td.Text()
			}
		})
		moas = append(moas, moa)
	})
	json.Moa = moas

	return nil
}

func handleInChIHashAndID(sibling *goquery.Selection, fieldPtr interface{}, name string) error {
	json, ok := fieldPtr.(*DrugInfo)
	if !ok {
		return fmt.Errorf("handleInChIHashAndID: type assertion to *DrugInfo failed")
	}

	if sibling == nil || json == nil {
		return fmt.Errorf("handleInChIHashAndID: invalid arguments")
	}

	content := sibling.Text()

	if name == "InchiKey" {
		json.InChI.Hash = content
	} else if name == "Inchi" {
		json.InChI.ID = content
	} else {
		return fmt.Errorf("handleInChIHashAndID: invalid name")
	}
	return nil
}

func handleMolecularWeight(sibling *goquery.Selection, fieldPtr interface{}, name string) error {
	json, ok := fieldPtr.(*DrugInfo)
	if !ok {
		return fmt.Errorf("handleMechanismOfAction: type assertion to *DrugInfo failed")
	}

	if sibling == nil || json == nil {
		return fmt.Errorf("handleMechanismOfAction: invalid arguments")
	}

	content := normalize(sibling.Text())
	parts := strings.Split(content, " ")

	if len(parts) == 4 {
		average, _ := strconv.ParseFloat(parts[1], 64)
		monoisotopic, _ := strconv.ParseFloat(parts[3], 64)

		json.Weight = append(json.Weight, MolWeight{
			Type:   "average",
			Weight: average,
			Units:  "Da",
		})
		json.Weight = append(json.Weight, MolWeight{
			Type:   "monoisotopic",
			Weight: monoisotopic,
			Units:  "Da",
		})
	} else {
		// handle error
		log.Printf("🔥 Error parsing molecular weight: %s\n", content)
	}
	return nil
}

var fieldHandlers = map[string]fieldHandler{
	"Synonyms":          handleListAsArray,
	"Categories":        handleListAsArray,
	"Groups":            handleListAsArray,
	"Description":       handleDescription,
	"Weight":            handleMolecularWeight,
	"Inchi":             handleInChIHashAndID,
	"InchiKey":          handleInChIHashAndID,
	"DrugInteractions":  handleDrugInteractions,
	"MechanismOfAction": handleMechanismOfAction,
	// Add other handlers here...
}

func setFieldByName(obj interface{}, fieldName string, newValue interface{}) error {
	// Get the reflect.Value of obj, which must be a pointer to a struct
	val := reflect.ValueOf(obj)
	if val.Kind() != reflect.Ptr || val.IsNil() {
		return fmt.Errorf("obj must be a non-nil pointer to a struct")
	}

	// Dereference the pointer to get the struct
	val = val.Elem()
	if val.Kind() != reflect.Struct {
		return fmt.Errorf("obj must be a pointer to a struct")
	}

	// Get the field by name
	fieldVal := val.FieldByName(fieldName)
	if !fieldVal.IsValid() {
		return fmt.Errorf("no such field: %s in obj", fieldName)
	}

	// Check if the field can be set
	if !fieldVal.CanSet() {
		return fmt.Errorf("cannot set field %s", fieldName)
	}

	// Convert newValue to a reflect.Value
	newFieldVal := reflect.ValueOf(newValue)

	// Check if the new value is assignable to the field
	if !newFieldVal.Type().AssignableTo(fieldVal.Type()) {
		return fmt.Errorf("provided value type did not match obj field type")
	}

	// Set the new value
	fieldVal.Set(newFieldVal)
	return nil
}

func assignField(drugInfo *DrugInfo, fieldString string, value string) {

	unhandledFields := map[string]interface{}{
		"isStub": &drugInfo.IsStub,
	}

	if strings.Contains(value, "Not Available") {
		fieldString = strings.ReplaceAll(fieldString, "Not Available", "")
		log.Printf("🔥 Field '%s' is not available, has contents: %s", fieldString, value)
		return
	}

	if fieldString == "AdverseEffects" {
		value = strings.ReplaceAll(value, "Improve decision support \u0026 research outcomesWith structured adverse effects data, including: blackbox warnings, adverse reactions, warning \u0026 precautions, \u0026 incidence rates. View sample adverse effects data in our new Data Library!See the data  Improve decision support \u0026 research outcomes with our structured adverse effects data.See a data sample", "")
	}

	if ptr, ok := unhandledFields[fieldString]; ok {
		switch v := ptr.(type) {
		case *bool:
			*v, _ = strconv.ParseBool(value)
		case *int:
			*v, _ = strconv.Atoi(value)
		case *float64:
			*v, _ = strconv.ParseFloat(value, 64)
		}
		return
	}

	var err error

	switch fieldString {
	case drugInfo.Description:
		err = setFieldByName(drugInfo, fieldString, ToTitleCase(value))
	case drugInfo.InChI.Hash:
		err = setFieldByName(drugInfo, fieldString, strings.ToUpper(value))
	case drugInfo.ID:
		err = setFieldByName(drugInfo, fieldString, strings.ToUpper(value))
	case drugInfo.IupacName:
		err = setFieldByName(drugInfo, fieldString, value)
	default:
		err = setFieldByName(drugInfo, fieldString, value)
	}

	if err != nil {
		log.Printf("🔥 Error assigning field '%s': %v", fieldString, err)
	}

}

func saveToFile(data interface{}, subdir string, filename string) {
	var path string

	// Create subdirectory if needed
	if subdir != "" {
		err := os.MkdirAll(subdir, 0755)
		if err != nil {
			log.Fatalf("Failed to create subdirectory: %s", err)
		}
		path = fmt.Sprintf("%s/%s.json", subdir, filename)
	} else {
		path = fmt.Sprintf("%s.json", filename)
	}

	// Create file
	file, err := os.Create(path)
	if err != nil {
		log.Fatalf("Failed to create file: %s", err)
	}
	defer file.Close()

	// Marshal data based on its type
	var jsonData []byte
	switch v := data.(type) {
	case []DrugInfo, DrugInfoStats:
		jsonData, err = json.MarshalIndent(v, "", "    ")
		if err != nil {
			log.Fatalf("Failed to marshal data: %s", err)
		}
	default:
		log.Fatalf("Unsupported data type")
	}

	// Write to file
	_, err = file.Write(jsonData)
	if err != nil {
		log.Fatalf("Failed to write to file: %s", err)
	}

	fmt.Printf("✅ Data saved to: ")
	absPath, _ := filepath.Abs(path)
	fmt.Printf("file://%s", absPath)
}

// New function for better error handling in goroutines
func scrapePageRoutine(pageLink DrugLink, wg *sync.WaitGroup, drugInfosChan chan<- DrugInfo) {
	defer wg.Done()
	defer func() {
		if r := recover(); r != nil {
			log.Println("Recovered in scrapePageRoutine:", r)
		}
	}()

	_, page, err := fetchPage(pageLink.Link)
	if err != nil {
		log.Printf("Error getting page: %v\n", err)
		return
	}

	json := DrugInfo{
		Link:        pageLink.Link,
		Description: "",
		Weight:      []MolWeight{},
	}

	// determine if the field is a string or []string
	var stringFields map[string]*string = ExtractFieldsOfType[string](&json)
	var stringSliceFields map[string]*[]string = ExtractFieldsOfType[[]string](&json)

	delete(stringSliceFields, "DrugInteractions")

	stubNoticeText := normalize(page.Find(".stub-notice").Children().Text())

	if stubNoticeText == normalize(STUB_NOTICE_TEXT) {
		json.IsStub = true
	} else {
		json.IsStub = false
	}

	// fmt print stub notice with emoji
	page.Find("dl").Find("dt").Each(func(_ int, s *goquery.Selection) {
		title := normalize(s.Text())
		sibling := s.Next()

		if sibling == nil {
			log.Println("🔥 sibling is nil")
			return
		}

		// If no handler exists, use assignField
		content := sibling.Text()
		if content == "Not Available" || content == "N/A" || normalize(content) == "not available" {
			return
		}

		propertyName := getFieldInfo(&json, title)

		if propertyName != "" {
			if handler, exists := fieldHandlers[propertyName]; exists {
				var err error
				fmt.Printf("ℹ️ Handling field '%s' with handler '%s'\n", propertyName, runtime.FuncForPC(reflect.ValueOf(handler).Pointer()).Name())

				if stringFields[propertyName] != nil {
					fieldPtr := stringFields[propertyName]
					err = handler(sibling, fieldPtr, propertyName)
				} else if stringSliceFields[propertyName] != nil {
					fieldPtr := stringSliceFields[propertyName]
					err = handler(sibling, fieldPtr, propertyName)
				} else {
					err = handler(sibling, &json, propertyName)
				}
				if err != nil {
					log.Printf("Error handling field '%s': %v", propertyName, err)
				}
				return
			} else {
				log.Printf("No handler found for field '%s', using assignField", propertyName)
				assignField(&json, propertyName, content)
				return
			}
		}
	})
	drugInfosChan <- json
}

type UniqueSet[K comparable, V any] map[K]V

func NewUniqueSet[K comparable, V any]() UniqueSet[K, V] {
	return make(UniqueSet[K, V])
}

func (s UniqueSet[K, V]) Add(key K, value V) {
	s[key] = value
}

func (s UniqueSet[K, V]) Contains(key K) bool {
	_, exists := s[key]
	return exists
}

func (s UniqueSet[K, V]) Get(key K) (V, bool) {
	val, exists := s[key]
	return val, exists
}

func (s UniqueSet[K, V]) Remove(key K) {
	delete(s, key)
}

type Stats struct {
	FieldCounts         UniqueSet[string, int]    `json:"fieldCounts"`
	AverageFieldLengths UniqueSet[string, int]    `json:"averageFieldLengths"`
	FieldCompleteness   UniqueSet[string, string] `json:"fieldCompleteness"`
	TotalStubs          int                       `json:"totalStubs"`
	TotalLength         int                       `json:"totalLength"`
}

type DuplicateEntries struct {
	IDSet UniqueSet[string, bool] `json:"set"`
	Total int                     `json:"total"`
}

type ValidDrugInfos struct {
	IDSet UniqueSet[string, bool] `json:"set"`
	Total int                     `json:"total"`
}
type DrugInfoStats struct {
	Stats                 Stats               `json:"stats"`
	DuplicateEntries      DuplicateEntries    `json:"duplicateEntries"`
	ValidDrugInfos        ValidDrugInfos      `json:"validDrugInfos"`
	ValidDrugInfosLengths UniqueSet[int, int] `json:"validDrugInfosLengths"`
	NumRetries            int                 `json:"numRetries"`
	NumRequests           int                 `json:"numRequests"`
	NumErrors             int                 `json:"numErrors"`
	NumSleeps             int                 `json:"numSleeps"`
	ErrorLog              []string            `json:"errorLog"`
}

// Define a struct to match the JSON structure
type DrugInteraction struct {
	Draw            int
	RecordsTotal    int `json:"recordsTotal"`
	RecordsFiltered int `json:"recordsFiltered"`
	Data            [][]string
}

// Function to parse the JSON and extract the data from the drug interactions JSON endpoint
// the url is of the form: {BASE_URL}/drugs/{id}/drug_interactions.json
//        w/ query params: ?start={p_start}&length={num_rows_return}&_={cache_timestamp}

// Function to parse the drug interactions
func ParseDrugInteractions(jsonData string) ([][]string, error) {
	var di DrugInteraction

	// Unmarshal the JSON data
	err := json.Unmarshal([]byte(jsonData), &di)
	if err != nil {
		return nil, err
	}

	var interactions [][]string
	for _, data := range di.Data {
		// Regular expression to match the <a> tags and extract the necessary parts
		re := regexp.MustCompile(`<a href="/drugs/([^"]+)">([^<]+)</a>`)
		matches := re.FindStringSubmatch(data[0])

		var resultArray []string
		if len(matches) >= 3 {
			drugID := matches[1]
			drugName := matches[2]

			resultArray = append(resultArray, drugID, drugName, data[1])
		}

		interactions = append(interactions, resultArray)
	}

	return interactions, nil
}

// Helper function to check the validity of a DrugInfo object
func isValidDrugInfo(drugInfo DrugInfo) bool {

	val := reflect.ValueOf(drugInfo)
	thisLength := reflect.TypeOf(drugInfo).NumField()

	if thisLength <= 4 {
		return false
	}

	numErrors := 0

	// check string fields
	if drugInfo.ID == "" || drugInfo.Molecule == "" || drugInfo.CAS == "" || drugInfo.IupacName == "" || drugInfo.Background == "" || drugInfo.Summary == "" || drugInfo.Formula == "" || drugInfo.Description == "" || drugInfo.Link == "" || drugInfo.Type == "" || drugInfo.Indication == "" || drugInfo.Pharmacodynamics == "" {
		// loop through all string fields
		for i := 0; i < thisLength; i++ {
			fieldValue := val.Field(i)
			if strVal, ok := fieldValue.Interface().(string); ok {
				if strVal == "" {
					numErrors++
				}
			}
		}
	}

	// check slice fields
	if len(drugInfo.Synonyms) == 0 || len(drugInfo.Weight) == 0 || len(drugInfo.Moa) == 0 || len(drugInfo.Categories) == 0 {
		return false
	}

	// check custom struct fields
	if drugInfo.InChI.ID == "" || drugInfo.InChI.Hash == "" {
		return false
	}

	return true
}

func main() {
	// get user input for page number
	var count, id int

	args := os.Args[1:]
	links := make([]DrugLink, 0)

	linksChan := make(chan []DrugLink)
	var wg_buildLinksSlice sync.WaitGroup

	if len(args) >= 1 {
		mode := args[0]
		switch mode {
		case "ID":
			id = getIntFromUserInput(fmt.Sprintf("Enter DB ID (max. %v)", MAX_PAGE))
			if id >= 0 && id <= 99999 {
				fmtId := fmt.Sprintf("DB0%v", id)
				thisLink := DrugLink{
					Name: fmtId,
					Link: fmt.Sprintf(DRUG_BASE_URL, fmtId),
				}
				links = append(links, thisLink)
			} else {
				panic(fmt.Sprintf("❌ Max ID value is 5 digits, i.e %v\n", 99999))
			}
		case "numPages":
			count = getIntFromUserInput(fmt.Sprintf("Enter number of pages to scrape (max. %v)", MAX_PAGE))

		}
	}

	if count != 0 {

		// check if count is within range
		if count > MAX_PAGE {
			fmt.Printf("❌ Max page is %v\n", MAX_PAGE)
			return
		}

		for i := 0; i < count; i++ {
			wg_buildLinksSlice.Add(1)
			pageNum := i // Capture the current value of i
			go func(pageNum int) {
				err := getPageByNumRoutine(pageNum, &wg_buildLinksSlice, linksChan)
				if err != nil {
					log.Printf("🔥 Error getting page: %v\n", err)
				}
			}(pageNum)
		}

		go func() {
			wg_buildLinksSlice.Wait()
			close(linksChan)
		}()

		for pageLinks := range linksChan {
			fmt.Printf("ℹ️ collected %v total links... \n", len(links))
			links = append(links, pageLinks...)
		}

	} else if id != 0 {
		fmt.Printf("Links: %v", links)
	}

	var wg_buildDrugInfoSlice sync.WaitGroup
	drugInfosChan := make(chan DrugInfo, len(links)) // Adjusted the buffer size

	for _, link := range links {
		wg_buildDrugInfoSlice.Add(1)
		go scrapePageRoutine(link, &wg_buildDrugInfoSlice, drugInfosChan)
	}

	wg_buildDrugInfoSlice.Wait()
	close(drugInfosChan)

	// Collect all data into a slice
	var drugInfos []DrugInfo

	drugInfoStats := DrugInfoStats{
		Stats: Stats{
			FieldCounts:         NewUniqueSet[string, int](),
			AverageFieldLengths: NewUniqueSet[string, int](),
			FieldCompleteness:   NewUniqueSet[string, string](),
			TotalStubs:          0,
			TotalLength:         0,
		},
		DuplicateEntries: DuplicateEntries{
			IDSet: NewUniqueSet[string, bool](),
			Total: 0,
		},
		ValidDrugInfos: ValidDrugInfos{
			IDSet: NewUniqueSet[string, bool](),
			Total: 0,
		},
		ValidDrugInfosLengths: NewUniqueSet[int, int](),
	}

	fieldLengths := make(map[string]int)

	for drugInfo := range drugInfosChan {

		// Update TotalLength
		drugInfoStats.Stats.TotalLength++

		// Check if it's a stub
		if drugInfo.IsStub {
			drugInfoStats.Stats.TotalStubs++
		}

		// Check for duplicates
		id := drugInfo.ID // Assuming ID is the unique identifier
		if _, exists := drugInfoStats.DuplicateEntries.IDSet.Get(id); !exists {
			drugInfoStats.DuplicateEntries.IDSet.Add(id, false)
		} else {
			if duplicate, _ := drugInfoStats.DuplicateEntries.IDSet.Get(id); !duplicate {
				drugInfoStats.DuplicateEntries.IDSet.Add(id, true) // Mark as duplicate
				drugInfoStats.DuplicateEntries.Total++
			}
		}

		// Check for valid DrugInfo
		isValid := isValidDrugInfo(drugInfo) // Implement this function based on your validity criteria
		if isValid {
			drugInfoStats.ValidDrugInfos.IDSet.Add(id, true)
			drugInfoStats.ValidDrugInfos.Total++
		}

		val := reflect.ValueOf(drugInfo)
		thisLength := reflect.TypeOf(drugInfo).NumField()
		fmt.Printf("Reflected kind: %v\n", val.Kind()) // Debugging line

		if val.Kind() == reflect.Ptr && !val.IsNil() {
			val = val.Elem()
			fmt.Printf("Dereferenced kind: %v\n", val.Kind()) // Debugging line
		}
		for i := 0; i < thisLength; i++ {
			fieldName := reflect.TypeOf(drugInfo).Field(i).Name
			fieldValue := val.Field(i)

			// Update FieldCounts
			currentCount, _ := drugInfoStats.Stats.FieldCounts.Get(fieldName)
			drugInfoStats.Stats.FieldCounts.Add(fieldName, currentCount+1)

			// Update field lengths for string fields
			if strVal, ok := fieldValue.Interface().(string); ok {
				fieldLengths[fieldName] += len(strVal)
			}
		}

		// Append the drugInfo to the slice
		drugInfos = append(drugInfos, drugInfo)
	}

	// Calculate averages and completeness after the loop
	for field, count := range drugInfoStats.Stats.FieldCounts {
		avgLength := 0
		if count > 0 {
			avgLength = fieldLengths[field] / count
		}
		drugInfoStats.Stats.AverageFieldLengths.Add(field, avgLength)
		completeness := fmt.Sprintf("%.2f%%", float64(count)/float64(len(drugInfos))*100)
		drugInfoStats.Stats.FieldCompleteness.Add(field, completeness)
	}

	drugInfoStats.NumErrors = stats_num_errors
	drugInfoStats.NumRequests = stats_num_requests
	drugInfoStats.NumRetries = stats_num_retry
	drugInfoStats.NumSleeps = stats_num_sleeps
	drugInfoStats.ErrorLog = stats_error_log

	PrettyPrint(drugInfos)
	PrettyPrint(drugInfoStats)

	// save debug data to file and also save the results to a file
	saveToFile(drugInfoStats, "logs", "drugInfoStats.json")
	saveToFile(drugInfos, "results", fmt.Sprintf("%d_len%d", time.Now().Unix(), len(drugInfos)))
}
