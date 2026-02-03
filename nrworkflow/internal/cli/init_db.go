package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"nrworkflow/internal/db"
)

var initDBCmd = &cobra.Command{
	Use:   "init-db",
	Short: "Initialize the database (creates nrworkflow.data in project root)",
	RunE: func(cmd *cobra.Command, args []string) error {
		database, err := db.OpenOrCreate(DataPath)
		if err != nil {
			return err
		}
		defer database.Close()

		// Always run InitSchema to ensure all tables exist
		// (OpenOrCreate only runs it for new databases)
		if err := database.InitSchema(); err != nil {
			return fmt.Errorf("failed to initialize schema: %w", err)
		}

		fmt.Printf("Database initialized at %s\n", database.Path)
		return nil
	},
}
