package evaluation

import "localize-agent/workflow/internal/contracts"

func readPackItems(files contracts.FileStore, path string) ([]packItem, error) {
	var root struct {
		Items []packItem `json:"items"`
	}
	if err := files.ReadJSON(path, &root); err != nil {
		return nil, err
	}
	return root.Items, nil
}
