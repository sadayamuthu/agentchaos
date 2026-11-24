// Command analyze turns pilot report.json files and run directories into
// statistics and LaTeX tables.
//
//	analyze --data paper/pilot/data [--labels paper/pilot/labels] --tex paper/generated
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

var stdout io.Writer = os.Stdout

func main() {
	data := flag.String("data", "", "directory of <model>/report.json")
	labels := flag.String("labels", "", "directory with labels.csv and key.csv (optional)")
	texDir := flag.String("tex", "", "output directory for rq*.tex")
	flag.Parse()
	if *data == "" || *texDir == "" {
		fmt.Fprintln(os.Stderr, "usage: analyze --data DIR [--labels DIR] --tex DIR")
		os.Exit(2)
	}
	if err := run(*data, *labels, *texDir); err != nil {
		fmt.Fprintln(os.Stderr, "analyze:", err)
		os.Exit(1)
	}
}

func run(data, labels, texDir string) error {
	cells, err := loadData(data, func(s string) { fmt.Fprintln(os.Stderr, "warning:", s) })
	if err != nil {
		return err
	}
	if len(cells) == 0 {
		return fmt.Errorf("no report.json found under %s", data)
	}
	if err := os.MkdirAll(texDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(texDir, "rq1.tex"), []byte(renderRQ1(cells)), 0o644); err != nil {
		return err
	}
	// Never leave a table from an earlier run behind when it is not regenerated.
	removeStale := func(name string) error {
		if err := os.Remove(filepath.Join(texDir, name)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := removeStale("rq2.tex"); err != nil {
		return err
	}
	if err := removeStale("rq3.tex"); err != nil {
		return err
	}
	if t := renderRQ2(cells); t != "" {
		if err := os.WriteFile(filepath.Join(texDir, "rq2.tex"), []byte(t), 0o644); err != nil {
			return err
		}
	}
	var agr *agreement
	var labelErr error
	if labels != "" {
		if agr, labelErr = loadAgreement(labels, data); labelErr == nil {
			labelErr = os.WriteFile(filepath.Join(texDir, "rq3.tex"), []byte(renderRQ3(agr)), 0o644)
		}
	}
	fmt.Fprint(stdout, summary(cells, agr))
	return labelErr
}
