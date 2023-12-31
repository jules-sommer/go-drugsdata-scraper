
## Go DrugBank Scraper

### Description
The Go DrugBank Scraper is a sophisticated ( ehm... messy but functional ) web scraping tool written in Go, targeting the DrugBank database to extract comprehensive drug information. It utilizes Go's concurrency features for efficient data retrieval and is equipped with functionalities to handle different modes of data scraping. ***it's also super hackable, this makes it a great tool for learning how to scrape data from the web using Go - as I did with it!! It was certainly a fun project and I hope you get something out of it too!*** Feel free to extend, PR, feature request, repurpose it entirely, or whatever you want to do with it. I'm open to any suggestions and contributions!

### Features
- **Concurrent Page Processing**: Employs Go's concurrency for efficient data scraping across multiple pages.
- **Customizable Querying**: Allows specification of page ranges or individual drug IDs for scraping.
- **Resilient and Intelligent**: Implements delay strategies to manage rate limiting and ensure uninterrupted scraping.
- **Detailed Data Extraction**: Gathers extensive information about drugs, including molecular details, pharmacodynamics, interactions, and more.
- **Output Serialization**: Provides scraped data in structured JSON format, ready for various applications.
- **Easy to Use**: Requires only a few command-line arguments to execute.
- **Extensible & Modular**: Easily extendable and modular for future updates. Example, handler functions for new data fields can be added to the \`main.go\` file, they are called automatically by the scraper depending on the data field being scraped.

### Installation
1. **Clone the Repository**:
   ```bash
   gh repo clone jules-sommer/go-drugsdata-scraper
   ```
   *or*
   ```bash
   git clone https://github.com/jules-sommer/go-drugsdata-scraper
   ```
   - I think the first is generally preferable on Unix machines, as it is on my copy of Ubuntu, and the later on Windows; but I don't rightly know why there are two commands and CLIs for git.
3. **Install Dependencies**:
   Navigate to the cloned directory and install necessary Go modules:
   ```bash
   cd go-drugbank-scraper
   go mod tidy
   ```

### Usage
Execute the scraper with command-line arguments to define the scraping mode:
1. **Scraping by Page Range**:
   ```bash
   go run main.go <MODE={ID,numPages}> <number_of_pages>
   ```
#### Modes
1. Page Range: [0-508]: As of writing, the maximum number of pages available for scraping is 508. There's a to-do in place to automatically detect the number of available pages in the future.
2. Single ID: use `ID` as the value for `MODE` cli arg, then enter the prompted value at runtime. Input the DrugBank ID as a 4-digit integer (e.g., 0123). The prefix "DB0" will be automatically added to the ID. As of now, specifying the prefix manually is not supported. Future updates will include features to fuzz the IDs and thus the pages to scrape.

    ***[NOTE]***: Currently the program prompts at runtime for the actual value of the ID or numPages args, this will be supported both ways in future.

### Contributing
I welcome contributions! For guidelines on how to contribute, please read our [CONTRIBUTING.md](CONTRIBUTING.md).

### License
This project is licensed under the MIT License - see the [LICENSE.md](LICENSE.md) file for details.

### Acknowledgments
- DrugBank for providing extensive drug information.
- The Go community for their invaluable resources and continuous support.
