package scaffold

import (
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Run is the entry point for the init-customer scaffold command.
// It parses args (--out, --name, --force, --git) and materializes the template.
func Run(args []string) error {
	fset := flag.NewFlagSet("init-customer", flag.ContinueOnError)
	out := fset.String("out", "", "output directory (required)")
	name := fset.String("name", "", "customer name for template substitution (default: basename of --out)")
	force := fset.Bool("force", false, "overwrite non-empty target directory")
	git := fset.Bool("git", false, "initialize a git repository after scaffolding")

	if err := fset.Parse(args); err != nil {
		return err
	}
	if *out == "" {
		return fmt.Errorf("--out is required")
	}
	if *name == "" {
		*name = filepath.Base(*out)
	}

	if !*force {
		entries, err := os.ReadDir(*out)
		if err == nil && len(entries) > 0 {
			return fmt.Errorf("target directory %q is not empty; use --force to overwrite", *out)
		}
	}

	if err := materialize(*out, *name); err != nil {
		return err
	}

	if *git {
		if err := initGit(*out); err != nil {
			// best-effort: print warning but don't fail
			fmt.Fprintf(os.Stderr, "warning: git init failed: %v\n", err)
		}
	}

	fmt.Printf("Customer config scaffolded at %s\n", *out)
	return nil
}

// materialize writes the embedded template tree into outDir,
// applying {{Name}} substitution to text files.
func materialize(outDir, name string) error {
	sub, err := fs.Sub(templateFS, "customer-config-template")
	if err != nil {
		return fmt.Errorf("template FS: %w", err)
	}

	vars := map[string]string{"Name": name}

	return fs.WalkDir(sub, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		dest := filepath.Join(outDir, filepath.FromSlash(path))

		if d.IsDir() {
			return os.MkdirAll(dest, 0o755)
		}

		data, err := fs.ReadFile(sub, path)
		if err != nil {
			return fmt.Errorf("read template %s: %w", path, err)
		}

		if isText(path) {
			data, err = Substitute(data, vars)
			if err != nil {
				return fmt.Errorf("substitute %s: %w", path, err)
			}
		}

		perm := fs.FileMode(0o644)
		if strings.HasSuffix(path, ".py") || strings.HasSuffix(path, ".sh") {
			perm = 0o755
		}

		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			return err
		}
		return os.WriteFile(dest, data, perm)
	})
}

// isText returns true for file extensions that should receive token substitution.
func isText(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".yaml", ".yml", ".json", ".md", ".txt", ".py", ".sh", ".toml", ".cfg", ".ini":
		return true
	default:
		return false
	}
}
