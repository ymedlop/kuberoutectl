// Command gen-docs renders the Markdown command reference for the docs site from
// the live Cobra command tree, so docs/reference/ can never drift from the CLI.
//
//	go run ./cmd/gen-docs [outdir]   # default outdir: docs/reference
//	make docs-reference
//
// Each page carries Just-the-Docs front matter (title, parent, and a per-command
// `description` from the command's Short — for SEO). Output is deterministic (no
// "auto generated on <date>" footer), so CI can fail a PR whose committed
// reference is stale — see the "docs-reference up to date" step in ci.yml.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/cobra/doc"
	"github.com/ymedlop/kuberoutectl/internal/cli"
)

func main() {
	outDir := "docs/reference"
	if len(os.Args) > 1 {
		outDir = os.Args[1]
	}

	root, err := cli.RootCommand()
	if err != nil {
		fail(err)
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fail(err)
	}
	// Clear previously generated command pages (but keep index.md and any other
	// hand-written files in the section), so a removed command leaves no stale page.
	stale, _ := filepath.Glob(filepath.Join(outDir, "kuberoutectl*.md"))
	for _, f := range stale {
		if err := os.Remove(f); err != nil {
			fail(err)
		}
	}

	n := genTree(root, outDir)
	fmt.Printf("wrote %d command pages to %s\n", n, outDir)
}

func genTree(cmd *cobra.Command, dir string) int {
	genOne(cmd, dir)
	n := 1
	for _, c := range cmd.Commands() {
		if !c.IsAvailableCommand() || c.IsAdditionalHelpTopicCommand() {
			continue
		}
		n += genTree(c, dir)
	}
	return n
}

func genOne(cmd *cobra.Command, dir string) {
	cmd.DisableAutoGenTag = true // stable output: no timestamped footer

	base := strings.ReplaceAll(cmd.CommandPath(), " ", "_") // kuberoutectl_target_use
	f, err := os.Create(filepath.Join(dir, base+".md"))
	if err != nil {
		fail(err)
	}
	defer f.Close()

	// Just-the-Docs front matter: flat "Command reference" section, alphabetical
	// by title (which keeps a command next to its subcommands), plus a per-page
	// SEO description from the command's one-line Short.
	fmt.Fprintf(f, "---\ntitle: %q\nparent: Command reference\nlayout: default\ndescription: %q\n---\n\n",
		cmd.CommandPath(), cmd.Short)

	// Identity link handler: leave same-dir .md links for jekyll-relative-links.
	if err := doc.GenMarkdownCustom(cmd, f, func(s string) string { return s }); err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "gen-docs:", err)
	os.Exit(1)
}
