package seeds

import (
	_ "embed"
	"encoding/json"
	"github.com/CallTag/backend/model"
)

//go:embed verified_numbers_seed.json
var verifiedSeedJSON []byte

// LoadVerifiedSeeds mempertahankan tanggal verifikasi asli tanpa memperbaruinya saat boot.
func LoadVerifiedSeeds() ([]model.VerifiedNumber, error) {
	var items []model.VerifiedNumber
	err := json.Unmarshal(verifiedSeedJSON, &items)
	return items, err
}
