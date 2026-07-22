package service

import (
	"fmt"
	"strings"
	"time"

	"be/internal/model"
	"be/internal/types"
)

func (s *CustomProviderService) Update(name string, req types.CustomProviderUpdateRequest) (*model.CustomProvider, error) {
	current, err := s.Get(name)
	if err != nil {
		return nil, err
	}

	updates := []string{}
	args := []any{}
	add := func(column string, value any) { updates = append(updates, column+" = ?"); args = append(args, value) }

	if req.BaseURL != nil {
		baseURL, validateErr := validateBaseURL(*req.BaseURL)
		if validateErr != nil {
			return nil, validateErr
		}
		add("base_url", baseURL)
	}
	if req.APIKey != nil {
		add("api_key", *req.APIKey)
	}
	if req.APIWire != nil {
		apiWire, validateErr := normalizeAPIWire(*req.APIWire)
		if validateErr != nil {
			return nil, validateErr
		}
		add("api_wire", apiWire)
	}
	if req.Enabled != nil {
		if !*req.Enabled {
			if err := s.InUseCheck(current.Name); err != nil {
				return nil, err
			}
		}
		if *req.Enabled {
			add("enabled", 1)
		} else {
			add("enabled", 0)
		}
	}
	if len(updates) == 0 {
		return current, nil
	}
	add("updated_at", s.clock.Now().UTC().Format(time.RFC3339Nano))
	args = append(args, current.Name)
	result, err := s.pool.Exec("UPDATE custom_providers SET "+strings.Join(updates, ", ")+" WHERE LOWER(name) = LOWER(?)", args...)
	if err != nil {
		return nil, err
	}
	if n, _ := result.RowsAffected(); n == 0 {
		return nil, fmt.Errorf("custom provider not found: %s", name)
	}
	return s.Get(current.Name)
}

func (s *CustomProviderService) Delete(name string) error {
	current, err := s.Get(name)
	if err != nil {
		return err
	}
	if err := s.InUseCheck(current.Name); err != nil {
		return err
	}
	_, err = s.pool.Exec("DELETE FROM custom_providers WHERE LOWER(name) = LOWER(?)", current.Name)
	return err
}

// InUseCheck blocks disabling or deleting a custom provider that any model row
// still references.
func (s *CustomProviderService) InUseCheck(name string) error {
	rows, err := s.pool.Query("SELECT id FROM models WHERE LOWER(provider) = LOWER(?)", name)
	if err != nil {
		return err
	}
	defer rows.Close()
	refs := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		refs = append(refs, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(refs) == 0 {
		return nil
	}
	return fmt.Errorf("custom provider is in use by: %s", strings.Join(refs, ", "))
}
