package deadlock

import "github.com/Eion21/mobatui/internal/progression"

func buildTestCatalog(api *APIClient) (*progression.Catalog, []string, error) {
	shop, _, err := api.shopItems()
	if err != nil {
		return nil, nil, err
	}
	raw := make([]progression.RawItem, 0, len(shop))
	for id, it := range shop {
		raw = append(raw, progression.RawItem{
			ID: id, Name: it.Name, Class: it.ClassName,
			Slot: it.ItemSlotType, Tier: it.ItemTier, Cost: it.Cost,
			Comps: it.ComponentItems, Shopable: true,
		})
	}
	return progression.BuildCatalog(raw)
}
