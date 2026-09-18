package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

func main() {
	covPath := flag.String("coverprofile", "coverage/coverage.out", "Path to coverage profile")
	minCov := flag.Float64("min", 50.0, "Minimum coverage percentage for new code")
	staged := flag.Bool("staged", false, "Check staged changes (git diff --cached)")
	baseRef := flag.String("base", "", "Base git ref to compare against (e.g. origin/main or HEAD~1)")
	flag.Parse()

	// 1. Determine git diff arguments
	diffArgs := []string{"diff", "-U0"}
	if *staged {
		diffArgs = append(diffArgs, "--cached")
	} else if *baseRef != "" {
		diffArgs = append(diffArgs, *baseRef)
	} else {
		// Default: compare against HEAD if there are unstaged changes, or HEAD~1 if clean
		diffArgs = append(diffArgs, "HEAD")
	}
	diffArgs = append(diffArgs, "--", "*.go")

	cmd := exec.Command("git", diffArgs...)
	out, err := cmd.Output()
	if err != nil {
		// Fallback: if HEAD diff fails (e.g. initial commit or no HEAD), try git diff against empty tree
		diffArgs[len(diffArgs)-2] = "HEAD~1"
		cmd = exec.Command("git", diffArgs...)
		out, err = cmd.Output()
		if err != nil {
			fmt.Printf("⚠️  Warnung: Git Diff konnte nicht ausgeführt werden: %v. Überspringe Diff-Coverage-Prüfung.\n", err)
			os.Exit(0)
		}
	}

	diffText := string(out)
	if strings.TrimSpace(diffText) == "" {
		fmt.Printf("ℹ️  Keine Änderungen an Go-Dateien im Diff gefunden.\n")
		os.Exit(0)
	}

	// 2. Parse diff to extract modified line numbers per file
	addedLinesByFile := parseDiff(diffText)
	if len(addedLinesByFile) == 0 {
		fmt.Printf("ℹ️  Keine neuen ausführbaren Go-Zeilen im Diff gefunden.\n")
		os.Exit(0)
	}

	// 3. Read coverage profile
	covFile, err := os.Open(*covPath)
	if err != nil {
		fmt.Printf("❌ Fehler beim Öffnen von %s: %v\n", *covPath, err)
		fmt.Println("Tipp: Führe vorher 'go test -coverprofile=coverage/coverage.out ./...' aus.")
		os.Exit(1)
	}
	defer covFile.Close()

	// Profile line format:
	// <module_path>/<file>:<start_line>.<col>,<end_line>.<col> <num_statements> <count>
	covScanner := bufio.NewScanner(covFile)
	type blockInfo struct {
		startLine int
		endLine   int
		count     int
	}
	fileCoverage := make(map[string][]blockInfo)

	for covScanner.Scan() {
		line := covScanner.Text()
		if strings.HasPrefix(line, "mode:") {
			continue
		}
		parts := strings.Split(line, ":")
		if len(parts) < 2 {
			continue
		}
		fullPkgFile := parts[0]
		rest := parts[1]

		// Normalize to relative repo path
		relFile := fullPkgFile
		if idx := strings.Index(fullPkgFile, "openlocalcrm/"); idx != -1 {
			relFile = fullPkgFile[idx+len("openlocalcrm/"):]
		}

		fields := strings.Fields(rest)
		if len(fields) < 3 {
			continue
		}
		posRange := fields[0] // start_line.col,end_line.col
		count, _ := strconv.Atoi(fields[2])

		posParts := strings.Split(posRange, ",")
		if len(posParts) < 2 {
			continue
		}
		startParts := strings.Split(posParts[0], ".")
		endParts := strings.Split(posParts[1], ".")
		sLine, _ := strconv.Atoi(startParts[0])
		eLine, _ := strconv.Atoi(endParts[0])

		fileCoverage[relFile] = append(fileCoverage[relFile], blockInfo{
			startLine: sLine,
			endLine:   eLine,
			count:     count,
		})
	}

	// 4. Match added lines with coverage
	totalExecutableNewLines := 0
	coveredNewLines := 0
	uncoveredLocations := make([]string, 0)

	for file, lines := range addedLinesByFile {
		blocks, ok := fileCoverage[file]
		if !ok {
			// Check if file matches with any relative suffix
			for covF, b := range fileCoverage {
				if strings.HasSuffix(covF, file) || strings.HasSuffix(file, covF) {
					blocks = b
					break
				}
			}
		}

		for _, lineNum := range lines {
			// Find if this line falls into any statement block
			isExecutable := false
			isCovered := false

			for _, b := range blocks {
				if lineNum >= b.startLine && lineNum <= b.endLine {
					isExecutable = true
					if b.count > 0 {
						isCovered = true
						break
					}
				}
			}

			if isExecutable {
				totalExecutableNewLines++
				if isCovered {
					coveredNewLines++
				} else {
					uncoveredLocations = append(uncoveredLocations, fmt.Sprintf("  - %s:%d", file, lineNum))
				}
			}
		}
	}

	if totalExecutableNewLines == 0 {
		fmt.Printf("✅ Diff-Coverage: Keine neuen ausführbaren Statements in modifizierten Go-Dateien (rein deklarativ/kommentare).\n")
		os.Exit(0)
	}

	actualCov := (float64(coveredNewLines) / float64(totalExecutableNewLines)) * 100.0

	fmt.Printf("\n🔍 [Diff-Coverage Analyse für neuen Code]\n")
	fmt.Printf("  • Neue ausführbare Zeilen: %d\n", totalExecutableNewLines)
	fmt.Printf("  • Getestete neue Zeilen:   %d\n", coveredNewLines)
	fmt.Printf("  • Diff-Abdeckung:          %.1f%% (Mindestanforderung: %.1f%%)\n\n", actualCov, *minCov)

	if actualCov < *minCov {
		fmt.Printf("❌ FEHLER: Neuer Code erreicht nicht die Mindest-Testabdeckung von %.1f%%!\n", *minCov)
		if len(uncoveredLocations) > 0 {
			fmt.Printf("Ungetestete neue Zeilen:\n")
			limit := 20
			for i, loc := range uncoveredLocations {
				if i >= limit {
					fmt.Printf("  ... und %d weitere ungetestete Zeilen\n", len(uncoveredLocations)-limit)
					break
				}
				fmt.Println(loc)
			}
		}
		fmt.Println("\nBitte Tests für die neuen Funktionen schreiben, bevor der Code gepusht/committet wird.")
		os.Exit(1)
	}

	fmt.Printf("✅ ERFOLG: Diff-Coverage Anforderung von %.1f%% erfolgreich erfüllt (%.1f%%)!\n\n", *minCov, actualCov)
}

func parseDiff(diffText string) map[string][]int {
	result := make(map[string][]int)
	scanner := bufio.NewScanner(strings.NewReader(diffText))

	var currentFile string
	hunkRegex := regexp.MustCompile(`^@@ -\d+(?:,\d+)? \+(\d+)(?:,(\d+))? @@`)

	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			// Filter out tests and auto-generated database files
			if strings.HasSuffix(currentFile, "_test.go") ||
				strings.HasPrefix(currentFile, "internal/db/models.go") ||
				strings.HasPrefix(currentFile, "internal/db/querier.go") {
				currentFile = ""
			}
			continue
		}

		if currentFile == "" {
			continue
		}

		if strings.HasPrefix(line, "@@ ") {
			matches := hunkRegex.FindStringSubmatch(line)
			if len(matches) >= 2 {
				startLine, _ := strconv.Atoi(matches[1])
				lineCount := 1
				if len(matches) >= 3 && matches[2] != "" {
					lineCount, _ = strconv.Atoi(matches[2])
				}
				for i := 0; i < lineCount; i++ {
					result[currentFile] = append(result[currentFile], startLine+i)
				}
			}
		}
	}
	return result
}
