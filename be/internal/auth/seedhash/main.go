// Throwaway tool that prints auth.Hash("admin") for one-time generation
// of the admin PHC literal pasted into 000078_seed_admin.up.sql.
// Not imported by main binaries.
package main

import (
	"fmt"
	"os"

	"be/internal/auth"
)

func main() {
	h, err := auth.Hash("admin")
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	fmt.Println(h)
}
