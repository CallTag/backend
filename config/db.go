package config

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"github.com/CallTag/backend/seeds"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	MongoClient    *mongo.Client
	MongoDB        *mongo.Database
	MongoConnected bool

	memMu               sync.RWMutex
	memUsers            = make(map[string]model.User)
	memPhoneTags        = make(map[string]model.PhoneTag)
	memSpamReports      = make(map[string][]model.SpamReport)
	memEmergency        []model.EmergencyDirectory
	memOfficialChannels []model.OfficialChannel
	memSyncCounter      int64 = 10
)

// InitDatabase menginisialisasi data seed memori dan menghubungkan ke MongoDB apabila MONGOSTRING tersedia.
func InitDatabase() {
	initMemorySeed()

	uri := GetMongoURI()
	if uri == "" {
		log.Println("[config/db] MONGOSTRING belum diset. Menggunakan penyimpanan in-memory yang telah di-seed untuk mode pengembangan.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		log.Printf("[config/db] Gagal membuat client MongoDB (%v), beralih ke fallback in-memory.\n", err)
		return
	}

	if err := client.Ping(ctx, nil); err != nil {
		log.Printf("[config/db] MongoDB tidak merespons ping (%v), beralih ke fallback in-memory.\n", err)
		return
	}

	MongoClient = client
	MongoDB = client.Database(GetDBName())
	MongoConnected = true
	log.Printf("[config/db] Terhubung ke MongoDB database: %s\n", GetDBName())

	if err := EnsureIndexesAndSeed(ctx); err != nil {
		log.Printf("[config/db] Peringatan saat membuat index/seed MongoDB: %v\n", err)
	}
}

func initMemorySeed() {
	memMu.Lock()
	defer memMu.Unlock()

	if emList, err := seeds.LoadEmergencySeeds(); err == nil {
		for i := range emList {
			emList[i].ID = primitive.NewObjectID()
		}
		memEmergency = emList
	}

	if chList, err := seeds.LoadOfficialChannelSeeds(); err == nil {
		for i := range chList {
			chList[i].ID = primitive.NewObjectID()
		}
		memOfficialChannels = chList
	}

	if scList, err := seeds.LoadScammerSeeds(); err == nil {
		for _, pt := range scList {
			pt.ID = primitive.NewObjectID()
			memPhoneTags[pt.PhoneE164] = pt
			if pt.SyncVersion > memSyncCounter {
				memSyncCounter = pt.SyncVersion
			}
			// Buat laporan awal representatif agar riwayat laporan nomor seed terlihat
			var seededReports []model.SpamReport
			for _, tc := range pt.Tags {
				seededReports = append(seededReports, model.SpamReport{
					ID:                    primitive.NewObjectID(),
					PhoneE164:             pt.PhoneE164,
					ReporterUserID:        "seed_community_" + tc.Tag,
					CategoryCode:          pt.PrimaryCategory,
					CustomTag:             tc.Tag,
					TagName:               tc.Tag,
					Comment:               "Laporan komunitas terverifikasi: " + tc.Tag,
					CallType:              "CELLULAR_OR_WA",
					ReporterTrustSnapshot: 1.0,
					AppliedWeight:         1.5,
					Status:                "VALIDATED",
					CreatedAt:             pt.LastReportedAt,
				})
			}
			memSpamReports[pt.PhoneE164] = seededReports
		}
	}
}

// EnsureIndexesAndSeed membuat index koleksi MongoDB sesuai PRD dan melakukan seed awal bila koleksi kosong.
func EnsureIndexesAndSeed(ctx context.Context) error {
	if !MongoConnected || MongoDB == nil {
		return nil
	}

	// 1. Index Users
	_, _ = MongoDB.Collection(ColUsers).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_hash", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_users_phone_hash"),
		},
		{
			Keys:    bson.D{{Key: "device_id_hash", Value: 1}},
			Options: options.Index().SetName("idx_users_device_hash"),
		},
	})

	// 2. Index PhoneTags
	_, _ = MongoDB.Collection(ColPhoneTags).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_phonetags_e164"),
		},
		{
			Keys:    bson.D{{Key: "reputation_level", Value: 1}, {Key: "sync_version", Value: 1}, {Key: "risk_score", Value: -1}},
			Options: options.Index().SetName("idx_phonetags_sync"),
		},
	})

	// 3. Index SpamReports
	_, _ = MongoDB.Collection(ColSpamReports).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}, {Key: "reporter_user_id", Value: 1}},
			Options: options.Index().SetName("idx_reports_dedup"),
		},
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_reports_phone_created"),
		},
	})

	// 4. Index EmergencyDirectory
	_, _ = MongoDB.Collection(ColEmergencyDirectory).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_emergency_code"),
		},
		{
			Keys:    bson.D{{Key: "scope", Value: 1}, {Key: "province", Value: 1}, {Key: "city_regency", Value: 1}, {Key: "agency_type", Value: 1}},
			Options: options.Index().SetName("idx_emergency_scope_region"),
		},
	})

	// 5. Index OfficialChannels
	_, _ = MongoDB.Collection(ColOfficialChannels).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "channel_slug", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_official_slug"),
		},
	})

	// Seed EmergencyDirectory
	emCol := MongoDB.Collection(ColEmergencyDirectory)
	if count, err := emCol.CountDocuments(ctx, bson.M{}); err == nil && count == 0 {
		if items, err := seeds.LoadEmergencySeeds(); err == nil {
			for _, item := range items {
				item.ID = primitive.NewObjectID()
				_, _ = emCol.InsertOne(ctx, item)
			}
			log.Printf("[config/db] Berhasil melakukan seed %d data EmergencyDirectory ke MongoDB.\n", len(items))
		}
	}

	// Seed OfficialChannels
	chCol := MongoDB.Collection(ColOfficialChannels)
	if count, err := chCol.CountDocuments(ctx, bson.M{}); err == nil && count == 0 {
		if items, err := seeds.LoadOfficialChannelSeeds(); err == nil {
			for _, item := range items {
				item.ID = primitive.NewObjectID()
				_, _ = chCol.InsertOne(ctx, item)
			}
			log.Printf("[config/db] Berhasil melakukan seed %d data OfficialChannels ke MongoDB.\n", len(items))
		}
	}

	// Seed PhoneTags (Top Scammers Awal)
	ptCol := MongoDB.Collection(ColPhoneTags)
	if count, err := ptCol.CountDocuments(ctx, bson.M{}); err == nil && count == 0 {
		if items, err := seeds.LoadScammerSeeds(); err == nil {
			for _, item := range items {
				item.ID = primitive.NewObjectID()
				_, _ = ptCol.InsertOne(ctx, item)
			}
			log.Printf("[config/db] Berhasil melakukan seed %d data awal PhoneTags ke MongoDB.\n", len(items))
		}
	}

	return nil
}

// FindOrCreateUser mencari pengguna berdasarkan phone_hash atau membuat pengguna baru saat login/registrasi.
func FindOrCreateUser(ctx context.Context, phoneE164, displayName, deviceID string) (model.User, error) {
	phoneHash := helper.HashIdentity(phoneE164)
	deviceHash := helper.HashIdentity(deviceID)
	now := time.Now().UTC()

	if strings.TrimSpace(displayName) == "" {
		displayName = "Warga CallTag Terverifikasi"
	}

	if MongoConnected && MongoDB != nil {
		col := MongoDB.Collection(ColUsers)
		var existing model.User
		err := col.FindOne(ctx, bson.M{"phone_hash": phoneHash}).Decode(&existing)
		if err == nil {
			existing.DisplayName = displayName
			existing.DeviceIDHash = deviceHash
			existing.UpdatedAt = now
			_, _ = col.UpdateOne(ctx, bson.M{"_id": existing.ID}, bson.M{
				"$set": bson.M{
					"display_name":   displayName,
					"device_id_hash": deviceHash,
					"updated_at":     now,
				},
			})
			return existing, nil
		}

		newUser := model.User{
			ID:           primitive.NewObjectID(),
			PhoneHash:    phoneHash,
			DeviceIDHash: deviceHash,
			DisplayName:  displayName,
			Role:         "USER",
			TrustWeight:  1.0,
			AccuracyStats: model.AccuracyStats{
				TotalReports:     0,
				ConfirmedReports: 0,
				DisputedReports:  0,
			},
			DailyReportCount: 0,
			Status:           "ACTIVE",
			CreatedAt:        now,
			UpdatedAt:        now,
		}
		if _, err := col.InsertOne(ctx, newUser); err != nil {
			return model.User{}, err
		}
		return newUser, nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	if u, exists := memUsers[phoneHash]; exists {
		u.DisplayName = displayName
		u.DeviceIDHash = deviceHash
		u.UpdatedAt = now
		memUsers[phoneHash] = u
		return u, nil
	}

	newUser := model.User{
		ID:           primitive.NewObjectID(),
		PhoneHash:    phoneHash,
		DeviceIDHash: deviceHash,
		DisplayName:  displayName,
		Role:         "USER",
		TrustWeight:  1.0,
		AccuracyStats: model.AccuracyStats{
			TotalReports:     0,
			ConfirmedReports: 0,
			DisputedReports:  0,
		},
		DailyReportCount: 0,
		Status:           "ACTIVE",
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	memUsers[phoneHash] = newUser
	return newUser, nil
}

// GetPhoneLookup mengambil data PhoneTag dan daftar laporan terbaru untuk nomor telepon tertentu.
func GetPhoneLookup(ctx context.Context, phoneE164 string) (*model.PhoneTag, []model.SpamReport, error) {
	if MongoConnected && MongoDB != nil {
		var pt model.PhoneTag
		err := MongoDB.Collection(ColPhoneTags).FindOne(ctx, bson.M{"phone_e164": phoneE164}).Decode(&pt)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, nil, nil
			}
			return nil, nil, err
		}
		pt.SpamScore = pt.RiskScore

		var reports []model.SpamReport
		opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: -1}}).SetLimit(10)
		cur, err := MongoDB.Collection(ColSpamReports).Find(ctx, bson.M{"phone_e164": phoneE164}, opts)
		if err == nil {
			_ = cur.All(ctx, &reports)
			_ = cur.Close(ctx)
			for i := range reports {
				if reports[i].TagName == "" {
					reports[i].TagName = reports[i].CustomTag
				}
			}
		}
		return &pt, reports, nil
	}

	memMu.RLock()
	defer memMu.RUnlock()

	pt, ok := memPhoneTags[phoneE164]
	if !ok {
		return nil, nil, nil
	}
	reps := memSpamReports[phoneE164]
	if len(reps) > 10 {
		reps = reps[len(reps)-10:]
	}
	return &pt, reps, nil
}

// CreateReportAndAggregate menyimpan laporan baru ke SpamReports dan mengagregasi ulang skor di PhoneTags.
func CreateReportAndAggregate(ctx context.Context, report model.SpamReport) (model.PhoneTag, error) {
	now := time.Now().UTC()
	report.ID = primitive.NewObjectID()
	report.Status = "VALIDATED"
	report.CreatedAt = now

	if MongoConnected && MongoDB != nil {
		repCol := MongoDB.Collection(ColSpamReports)
		if _, err := repCol.InsertOne(ctx, report); err != nil {
			return model.PhoneTag{}, err
		}

		// Ambil seluruh laporan untuk nomor ini guna menghitung ulang agregasi Risk Score & Tag
		cur, err := repCol.Find(ctx, bson.M{"phone_e164": report.PhoneE164})
		if err != nil {
			return model.PhoneTag{}, err
		}
		var allReports []model.SpamReport
		if err := cur.All(ctx, &allReports); err != nil {
			_ = cur.Close(ctx)
			return model.PhoneTag{}, err
		}
		_ = cur.Close(ctx)

		ptCol := MongoDB.Collection(ColPhoneTags)
		var existing model.PhoneTag
		var existingPtr *model.PhoneTag
		if err := ptCol.FindOne(ctx, bson.M{"phone_e164": report.PhoneE164}).Decode(&existing); err == nil {
			existingPtr = &existing
		}

		nextSync := time.Now().Unix()
		aggregated := helper.AggregatePhoneTag(existingPtr, allReports, nextSync)

		upsertOpts := options.Update().SetUpsert(true)
		_, err = ptCol.UpdateOne(
			ctx,
			bson.M{"phone_e164": report.PhoneE164},
			bson.M{"$set": bson.M{
				"phone_e164":              aggregated.PhoneE164,
				"country_code":            aggregated.CountryCode,
				"carrier":                 aggregated.Carrier,
				"reputation_level":        aggregated.ReputationLevel,
				"risk_score":              aggregated.SpamScore,
				"primary_category":        aggregated.PrimaryCategory,
				"primary_category_label":  aggregated.PrimaryCategoryLabel,
				"is_danger":               aggregated.IsDanger,
				"top_community_label":     aggregated.TopCommunityLabel,
				"tags":                    aggregated.Tags,
				"category_breakdown":      aggregated.CategoryBreakdown,
				"unique_reporters":        aggregated.UniqueReporters,
				"total_reports":           aggregated.TotalReports,
				"is_official_blacklisted": aggregated.IsOfficialBlacklisted,
				"is_whitelisted_verified": aggregated.IsWhitelistedVerified,
				"sync_version":            aggregated.SyncVersion,
				"first_reported_at":       aggregated.FirstReportedAt,
				"last_reported_at":        aggregated.LastReportedAt,
				"updated_at":              aggregated.UpdatedAt,
			}},
			upsertOpts,
		)
		if err != nil {
			return model.PhoneTag{}, err
		}
		return aggregated, nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	existing, hasExisting := memPhoneTags[report.PhoneE164]
	var existingPtr *model.PhoneTag
	if hasExisting {
		existingPtr = &existing
	}

	// Jika nomor berasal dari seed dan baru memiliki sedikit laporan sintetis, gabungkan dengan laporan baru
	allReports := append(memSpamReports[report.PhoneE164], report)
	memSpamReports[report.PhoneE164] = allReports

	memSyncCounter++
	aggregated := helper.AggregatePhoneTag(existingPtr, allReports, memSyncCounter)
	if aggregated.ID.IsZero() {
		aggregated.ID = primitive.NewObjectID()
	}
	memPhoneTags[report.PhoneE164] = aggregated
	return aggregated, nil
}

// QueryEmergency mengambil daftar nomor darurat nasional dan direktori lokal dengan filter province, city, dan agency_type.
func QueryEmergency(ctx context.Context, province, city, agencyType string) ([]model.EmergencyDirectory, []model.EmergencyDirectory, error) {
	provFilter := strings.ToLower(strings.TrimSpace(province))
	cityFilter := strings.ToLower(strings.TrimSpace(city))
	typeFilter := strings.ToUpper(strings.TrimSpace(agencyType))

	var allItems []model.EmergencyDirectory

	if MongoConnected && MongoDB != nil {
		cur, err := MongoDB.Collection(ColEmergencyDirectory).Find(ctx, bson.M{})
		if err != nil {
			return nil, nil, err
		}
		if err := cur.All(ctx, &allItems); err != nil {
			_ = cur.Close(ctx)
			return nil, nil, err
		}
		_ = cur.Close(ctx)
	} else {
		memMu.RLock()
		allItems = append(allItems, memEmergency...)
		memMu.RUnlock()
	}

	var national []model.EmergencyDirectory
	var local []model.EmergencyDirectory

	for _, item := range allItems {
		if item.Scope == "NATIONAL" {
			if typeFilter == "" || strings.EqualFold(item.AgencyType, typeFilter) {
				national = append(national, item)
			}
			continue
		}

		if provFilter != "" && !strings.Contains(strings.ToLower(item.Province), provFilter) {
			continue
		}
		if cityFilter != "" && !strings.Contains(strings.ToLower(item.CityRegency), cityFilter) {
			continue
		}
		if typeFilter != "" && !strings.EqualFold(item.AgencyType, typeFilter) {
			continue
		}
		local = append(local, item)
	}

	return national, local, nil
}

// GetOfficialChannelsByCategory mengambil kanal pelaporan resmi pemerintah (opsional difilter berdasarkan kategori scam).
func GetOfficialChannelsByCategory(ctx context.Context, category model.CategoryCode) ([]model.OfficialChannel, error) {
	var all []model.OfficialChannel

	if MongoConnected && MongoDB != nil {
		opts := options.Find().SetSort(bson.D{{Key: "display_order", Value: 1}})
		cur, err := MongoDB.Collection(ColOfficialChannels).Find(ctx, bson.M{"is_active": true}, opts)
		if err != nil {
			return nil, err
		}
		_ = cur.All(ctx, &all)
		_ = cur.Close(ctx)
	} else {
		memMu.RLock()
		all = append(all, memOfficialChannels...)
		memMu.RUnlock()
	}

	if category == "" {
		return all, nil
	}

	var filtered []model.OfficialChannel
	for _, ch := range all {
		for _, rel := range ch.RelatedCategories {
			if rel == category {
				filtered = append(filtered, ch)
				break
			}
		}
	}
	if len(filtered) == 0 {
		return all, nil
	}
	return filtered, nil
}

// GetTopScammersForSync mengambil daftar nomor SPAM & DANGER_SCAM untuk sinkronisasi Room DB / SQLite lokal di Android.
func GetTopScammersForSync(ctx context.Context, sinceVersion int64, limit int) ([]model.SpamSyncItem, int64, error) {
	if limit <= 0 || limit > 1000 {
		limit = 200
	}

	var tags []model.PhoneTag

	if MongoConnected && MongoDB != nil {
		filter := bson.M{
			"sync_version": bson.M{"$gt": sinceVersion},
			"reputation_level": bson.M{
				"$in": []model.ReputationLevel{model.ReputationDangerScam, model.ReputationSpam, model.ReputationSuspicious},
			},
		}
		opts := options.Find().SetSort(bson.D{{Key: "risk_score", Value: -1}}).SetLimit(int64(limit))
		cur, err := MongoDB.Collection(ColPhoneTags).Find(ctx, filter, opts)
		if err != nil {
			return nil, sinceVersion, err
		}
		_ = cur.All(ctx, &tags)
		_ = cur.Close(ctx)
	} else {
		memMu.RLock()
		for _, pt := range memPhoneTags {
			if pt.SyncVersion > sinceVersion && pt.ReputationLevel != model.ReputationClean {
				tags = append(tags, pt)
			}
		}
		memMu.RUnlock()

		sort.Slice(tags, func(i, j int) bool {
			return tags[i].SpamScore > tags[j].SpamScore
		})
		if len(tags) > limit {
			tags = tags[:limit]
		}
	}

	var latestVer = sinceVersion
	var syncItems []model.SpamSyncItem
	for _, pt := range tags {
		if pt.SyncVersion > latestVer {
			latestVer = pt.SyncVersion
		}
		score := pt.RiskScore
		if score == 0 {
			score = pt.SpamScore
		}
		syncItems = append(syncItems, model.SpamSyncItem{
			PhoneE164:            pt.PhoneE164,
			ReputationLevel:      pt.ReputationLevel,
			RiskScore:            score,
			SpamScore:            score,
			IsDanger:             pt.IsDanger,
			PrimaryCategory:      pt.PrimaryCategory,
			PrimaryCategoryLabel: pt.PrimaryCategoryLabel,
			TopCommunityLabel:    pt.TopCommunityLabel,
			SyncVersion:          pt.SyncVersion,
			UpdatedAt:            pt.UpdatedAt,
		})
	}

	if syncItems == nil {
		syncItems = []model.SpamSyncItem{}
	}
	return syncItems, latestVer, nil
}

// ComputePublicStats menghitung statistik agregat publik untuk GET /api/v1/stats/public.
func ComputePublicStats(ctx context.Context) model.PublicStatsResponse {
	catTotals := map[string]int{
		string(model.CategoryPinjolDC):      0,
		string(model.CategoryAPKSniffing):   0,
		string(model.CategoryImpersonation): 0,
		string(model.CategoryJudol):         0,
		string(model.CategorySocialEng):     0,
		string(model.CategoryTelemarketing): 0,
	}

	var allTags []model.PhoneTag
	if MongoConnected && MongoDB != nil {
		if cur, err := MongoDB.Collection(ColPhoneTags).Find(ctx, bson.M{}); err == nil {
			_ = cur.All(ctx, &allTags)
			_ = cur.Close(ctx)
		}
	} else {
		memMu.RLock()
		for _, pt := range memPhoneTags {
			allTags = append(allTags, pt)
		}
		memMu.RUnlock()
	}

	var dangerCount, spamCount, suspiciousCount int
	for _, pt := range allTags {
		switch pt.ReputationLevel {
		case model.ReputationDangerScam:
			dangerCount++
		case model.ReputationSpam:
			spamCount++
		case model.ReputationSuspicious:
			suspiciousCount++
		}
		for k, v := range pt.CategoryBreakdown {
			catTotals[k] += v
		}
	}

	nat, loc, _ := QueryEmergency(ctx, "", "", "")
	ch, _ := GetOfficialChannelsByCategory(ctx, "")

	return model.PublicStatsResponse{
		TotalTrackedNumbers:   len(allTags),
		DangerScamCount:       dangerCount,
		SpamCount:             spamCount,
		SuspiciousCount:       suspiciousCount,
		EmergencyAgencies:     len(nat) + len(loc),
		OfficialChannelsCount: len(ch),
		CategoryTotals:        catTotals,
		UpdatedAt:             time.Now().UTC(),
	}
}
