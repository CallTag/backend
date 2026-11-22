package config

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"sort"
	"strconv"
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
	ErrQuotaExceeded       = errors.New("batas kuota terlampaui")
	ErrDuplicateDispute    = errors.New("sanggahan aktif untuk nomor ini sudah ada")
	ErrNotFound            = errors.New("data tidak ditemukan")
	ErrIdempotencyMismatch = errors.New("Idempotency-Key telah digunakan untuk nomor yang berbeda")
	ErrInvalidCursor       = errors.New("format cursor sinkronisasi tidak valid")
	ErrFullResyncRequired  = errors.New("FULL_RESYNC_REQUIRED")
	ErrInvalidRefreshToken = errors.New("refresh token tidak valid atau telah dicabut")
	ErrUserInactive        = errors.New("akun tidak aktif atau telah dihapus")
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
	memModerationCases  = make(map[string]model.ModerationCase)
	memSyncChanges      []model.SyncChange
	memSyncSeq          int64
	memSyncGen                = 1
	memRefreshTokens          = make(map[string]model.RefreshTokenDoc)
	memSyncCounter      int64 = 10
)

// InitDatabase memvalidasi konfigurasi keamanan dan menghubungkan ke MongoDB atau seed in-memory (mode dev).
func InitDatabase() {
	if err := ValidateConfig(); err != nil {
		log.Fatalf("[config/db] %v\n", err)
	}

	if !IsProdEnv() {
		initMemorySeed()
	}

	uri := GetMongoURI()
	if uri == "" {
		if IsProdEnv() {
			log.Fatalln("[config/db] MONGOSTRING wajib diset pada mode produksi.")
		}
		log.Println("[config/db] MONGOSTRING belum diset. Menggunakan penyimpanan in-memory yang telah di-seed untuk mode pengembangan.")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	clientOpts := options.Client().ApplyURI(uri)
	client, err := mongo.Connect(ctx, clientOpts)
	if err != nil {
		if IsProdEnv() {
			log.Fatalf("[config/db] Gagal terhubung ke MongoDB pada mode produksi: %v\n", err)
		}
		log.Printf("[config/db] Gagal membuat client MongoDB (%v), beralih ke fallback in-memory.\n", err)
		return
	}

	if err := client.Ping(ctx, nil); err != nil {
		if IsProdEnv() {
			log.Fatalf("[config/db] MongoDB tidak merespons ping pada mode produksi: %v\n", err)
		}
		log.Printf("[config/db] MongoDB tidak merespons ping (%v), beralih ke fallback in-memory.\n", err)
		return
	}

	MongoClient = client
	MongoDB = client.Database(GetDBName())
	MongoConnected = true
	log.Printf("[config/db] Terhubung ke MongoDB database: %s (APP_ENV=%s)\n", GetDBName(), GetAppEnv())

	if err := EnsureIndexesAndSeed(ctx); err != nil {
		if IsProdEnv() {
			log.Fatalf("[config/db] Gagal menyiapkan index MongoDB pada mode produksi: %v\n", err)
		}
		log.Printf("[config/db] Peringatan saat membuat index/seed MongoDB: %v\n", err)
	}
}

// ResetMemoryForTest mengembalikan seluruh state in-memory ke kondisi seed awal yang bersih untuk unit/integration test.
func ResetMemoryForTest() {
	ResetStaffForTest()
	memMu.Lock()
	MongoConnected = false
	MongoClient = nil
	MongoDB = nil
	memUsers = make(map[string]model.User)
	memPhoneTags = make(map[string]model.PhoneTag)
	memSpamReports = make(map[string][]model.SpamReport)
	memEmergency = nil
	memOfficialChannels = nil
	memVerifiedNumbers = make(map[string]model.VerifiedNumber)
	memModerationCases = make(map[string]model.ModerationCase)
	memSyncChanges = nil
	memSyncSeq = 0
	memSyncGen = 1
	memRefreshTokens = make(map[string]model.RefreshTokenDoc)
	memSyncCounter = 10
	ResetModerationMemoryLocked()
	memMu.Unlock()

	initMemorySeed()
}

// PingDatabase memverifikasi kesiapan penyimpanan data untuk endpoint /readyz (T-107).
func PingDatabase(ctx context.Context) error {
	if MongoConnected && MongoClient != nil {
		pingCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
		defer cancel()
		return MongoClient.Ping(pingCtx, nil)
	}
	if IsProdEnv() {
		return errors.New("koneksi MongoDB belum aktif pada mode produksi")
	}
	if strings.TrimSpace(GetMongoURI()) != "" {
		return errors.New("MongoDB terkonfigurasi tetapi koneksi tidak aktif")
	}

	return nil
}

func initMemorySeed() {
	memMu.Lock()
	defer memMu.Unlock()
	initVerifiedMemorySeedLocked()

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
			if pt.LastReportedAt.IsZero() {
				pt.LastReportedAt = time.Now().UTC()
			}
			pt.LastActivityAt = pt.LastReportedAt
			pt.ExpiresAt = pt.LastReportedAt.Add(time.Duration(helper.ReportMaxAgeDays) * 24 * time.Hour)
			memPhoneTags[pt.PhoneE164] = pt
			if pt.SyncVersion > memSyncCounter {
				memSyncCounter = pt.SyncVersion
			}
			appendSyncChangeMemoryLocked("UPSERT", "PHONE_TAG", pt.PhoneE164, buildPhoneTagSyncPayload(pt))
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
	if err := EnsureStaffIndexes(ctx); err != nil {
		return err
	}
	if err := EnsureAuditIndexes(ctx); err != nil {
		return err
	}
	if err := EnsureModerationIndexes(ctx); err != nil {
		return err
	}

	if err := EnsureVerifiedIndexesAndSeed(ctx); err != nil {
		return err
	}

	// 1. Index Users (sparse unique agar registrasi berbasis install_id tanpa nomor HP tidak bentrok)
	userIndexes := []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_hash", Value: 1}},
			Options: options.Index().SetUnique(true).SetSparse(true).SetName("idx_users_phone_hash"),
		},
		{
			Keys:    bson.D{{Key: "device_id_hash", Value: 1}},
			Options: options.Index().SetName("idx_users_device_hash"),
		},
	}
	if _, err := MongoDB.Collection(ColUsers).Indexes().CreateMany(ctx, userIndexes); err != nil {
		_, _ = MongoDB.Collection(ColUsers).Indexes().DropOne(ctx, "idx_users_phone_hash")
		_, _ = MongoDB.Collection(ColUsers).Indexes().CreateMany(ctx, userIndexes)
	}

	if _, err := MongoDB.Collection(ColUsers).Indexes().CreateOne(ctx, mongo.IndexModel{
		Keys:    bson.D{{Key: "google_sub", Value: 1}},
		Options: options.Index().SetUnique(true).SetSparse(true).SetName("idx_users_google_sub"),
	}); err != nil {
		return err
	}

	// 2. Index PhoneTags
	if _, err := MongoDB.Collection(ColPhoneTags).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_phonetags_e164"),
		},
		{
			Keys:    bson.D{{Key: "reputation_level", Value: 1}, {Key: "sync_version", Value: 1}, {Key: "risk_score", Value: -1}},
			Options: options.Index().SetName("idx_phonetags_sync"),
		},
	}); err != nil {
		return err
	}

	// 3. Index SpamReports (termasuk unique index idempotency_key & jendela dedup 30 hari T-104)
	if _, err := MongoDB.Collection(ColSpamReports).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}, {Key: "reporter_user_id", Value: 1}},
			Options: options.Index().SetName("idx_reports_dedup"),
		},
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}, {Key: "created_at", Value: -1}},
			Options: options.Index().SetName("idx_reports_phone_created"),
		},
		{
			Keys: bson.D{{Key: "reporter_user_id", Value: 1}, {Key: "idempotency_key", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("idx_reports_idempotency").
				SetPartialFilterExpression(bson.M{"idempotency_key": bson.M{"$gt": ""}}),
		},
		{
			Keys: bson.D{{Key: "phone_e164", Value: 1}, {Key: "reporter_user_id", Value: 1}, {Key: "window_key", Value: 1}},
			Options: options.Index().
				SetUnique(true).
				SetName("idx_reports_dedup_window").
				SetPartialFilterExpression(bson.M{"window_key": bson.M{"$gt": 0}}),
		},
	}); err != nil {
		return err
	}

	// 4. Index ModerationCases (sanggahan T-105)
	if _, err := MongoDB.Collection(ColModerationCases).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "dispute_id", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_modcases_dispute_id"),
		},
		{
			Keys:    bson.D{{Key: "phone_e164", Value: 1}, {Key: "submitter_id", Value: 1}, {Key: "status", Value: 1}},
			Options: options.Index().SetName("idx_modcases_active_lookup"),
		},
		{
			Keys:    bson.D{{Key: "submitter_id", Value: 1}, {Key: "submitted_at", Value: -1}},
			Options: options.Index().SetName("idx_modcases_submitter_time"),
		},
	}); err != nil {
		return err
	}

	// 5. Index EmergencyDirectory
	if _, err := MongoDB.Collection(ColEmergencyDirectory).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "code", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_emergency_code"),
		},
		{
			Keys:    bson.D{{Key: "scope", Value: 1}, {Key: "province", Value: 1}, {Key: "city_regency", Value: 1}, {Key: "agency_type", Value: 1}},
			Options: options.Index().SetName("idx_emergency_scope_region"),
		},
	}); err != nil {
		return err
	}

	// 6. Index OfficialChannels
	if _, err := MongoDB.Collection(ColOfficialChannels).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "channel_slug", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_official_slug"),
		},
	}); err != nil {
		return err
	}

	// 7. Index SyncChanges (T-201)
	if _, err := MongoDB.Collection(ColSyncChanges).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "generation", Value: 1}, {Key: "seq", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_sync_gen_seq"),
		},
	}); err != nil {
		return err
	}

	// 8. Index RefreshTokens (T-204)
	if _, err := MongoDB.Collection(ColRefreshTokens).Indexes().CreateMany(ctx, []mongo.IndexModel{
		{
			Keys:    bson.D{{Key: "token_hash", Value: 1}},
			Options: options.Index().SetUnique(true).SetName("idx_refresh_token_hash"),
		},
		{
			Keys:    bson.D{{Key: "user_id", Value: 1}},
			Options: options.Index().SetName("idx_refresh_user_id"),
		},
		{
			Keys:    bson.D{{Key: "expires_at", Value: 1}},
			Options: options.Index().SetExpireAfterSeconds(0).SetName("idx_refresh_ttl"),
		},
	}); err != nil {
		return err
	}

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
				if item.LastReportedAt.IsZero() {
					item.LastReportedAt = time.Now().UTC()
				}
				item.LastActivityAt = item.LastReportedAt
				item.ExpiresAt = item.LastReportedAt.Add(time.Duration(helper.ReportMaxAgeDays) * 24 * time.Hour)
				_, _ = ptCol.InsertOne(ctx, item)
				_, _ = appendSyncChangeMongo(ctx, "UPSERT", "PHONE_TAG", item.PhoneE164, buildPhoneTagSyncPayload(item))
			}
			log.Printf("[config/db] Berhasil melakukan seed %d data awal PhoneTags ke MongoDB.\n", len(items))
		}
	}

	// Pastikan operasi admin/moderasi yang terhenti sebelum selesai pulih otomatis saat startup.
	if err := recoverAdminStartup(ctx); err != nil {
		return err
	}
	if err := RecoverModerationDecisions(ctx); err != nil {
		return err
	}

	return nil
}

// FindOrCreateUser mencari pengguna berdasarkan phone_hash atau device_id_hash (OD-02) atau membuat pengguna baru.
func FindOrCreateUser(ctx context.Context, phoneE164, displayName, deviceID string) (model.User, error) {
	deviceHash := helper.HashIdentity(deviceID)
	phoneHash := ""
	phoneMask := "Perangkat Terdaftar"
	if strings.TrimSpace(phoneE164) != "" {
		phoneHash = helper.HashIdentity(phoneE164)
		phoneMask = helper.MaskPhone(phoneE164)
	}
	now := time.Now().UTC()

	if strings.TrimSpace(displayName) == "" {
		displayName = "Warga CallTag Terverifikasi"
	}

	if MongoConnected && MongoDB != nil {
		col := MongoDB.Collection(ColUsers)
		filter := bson.M{"device_id_hash": deviceHash}
		if phoneHash != "" {
			filter = bson.M{"phone_hash": phoneHash}
		}

		var existing model.User
		err := col.FindOne(ctx, filter).Decode(&existing)
		if err == nil {
			existing.DisplayName = displayName
			existing.DeviceIDHash = deviceHash
			if existing.PhoneMask == "" {
				existing.PhoneMask = phoneMask
			}
			if existing.TokenVersion <= 0 {
				existing.TokenVersion = 1
			}
			existing.UpdatedAt = now
			_, _ = col.UpdateOne(ctx, bson.M{"_id": existing.ID}, bson.M{
				"$set": bson.M{
					"display_name":   displayName,
					"device_id_hash": deviceHash,
					"phone_mask":     existing.PhoneMask,
					"token_version":  existing.TokenVersion,
					"updated_at":     now,
				},
			})
			return existing, nil
		}

		newUser := model.User{
			ID:           primitive.NewObjectID(),
			PhoneMask:    phoneMask,
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
			TokenVersion:     1,
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

	memKey := "dev:" + deviceHash
	if phoneHash != "" {
		memKey = phoneHash
	}

	if u, exists := memUsers[memKey]; exists {
		u.DisplayName = displayName
		u.DeviceIDHash = deviceHash
		if u.PhoneMask == "" {
			u.PhoneMask = phoneMask
		}
		if u.TokenVersion <= 0 {
			u.TokenVersion = 1
		}
		u.UpdatedAt = now
		memUsers[memKey] = u
		return u, nil
	}

	newUser := model.User{
		ID:           primitive.NewObjectID(),
		PhoneMask:    phoneMask,
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
		TokenVersion:     1,
		CreatedAt:        now,
		UpdatedAt:        now,
	}
	memUsers[memKey] = newUser
	return newUser, nil
}

// RedactReportsForPublic membuat salinan slice SpamReport tanpa field privat pelapor maupun kunci idempotensi (T-106).
func RedactReportsForPublic(reports []model.SpamReport) []model.SpamReport {
	if len(reports) == 0 {
		return []model.SpamReport{}
	}
	out := make([]model.SpamReport, len(reports))
	for i, rep := range reports {
		rep.ReporterUserID = ""
		rep.IdempotencyKey = ""
		rep.WindowKey = 0
		if rep.TagName == "" {
			rep.TagName = rep.CustomTag
		}
		out[i] = rep
	}
	return out
}

// GetPhoneLookup mengambil data PhoneTag dan daftar laporan terbaru yang sudah teredaksi untuk nomor telepon tertentu.
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
		}
		return &pt, RedactReportsForPublic(reports), nil
	}

	memMu.RLock()
	defer memMu.RUnlock()

	pt, ok := memPhoneTags[phoneE164]
	if !ok {
		return nil, nil, nil
	}
	src := memSpamReports[phoneE164]
	copied := make([]model.SpamReport, len(src))
	copy(copied, src)
	sort.Slice(copied, func(i, j int) bool {
		return copied[i].CreatedAt.After(copied[j].CreatedAt)
	})
	if len(copied) > 10 {
		copied = copied[:10]
	}
	return &pt, RedactReportsForPublic(copied), nil
}

func phoneTagBsonSet(aggregated model.PhoneTag) bson.M {
	return bson.M{
		"phone_e164":                 aggregated.PhoneE164,
		"country_code":               aggregated.CountryCode,
		"carrier":                    aggregated.Carrier,
		"reputation_level":           aggregated.ReputationLevel,
		"risk_score":                 aggregated.SpamScore,
		"primary_category":           aggregated.PrimaryCategory,
		"primary_category_label":     aggregated.PrimaryCategoryLabel,
		"is_danger":                  aggregated.IsDanger,
		"moderator_confirmed_danger": aggregated.ModeratorConfirmedDanger,
		"pending_danger_review":      aggregated.PendingDangerReview,
		"top_community_label":        aggregated.TopCommunityLabel,
		"tags":                       aggregated.Tags,
		"category_breakdown":         aggregated.CategoryBreakdown,
		"unique_reporters":           aggregated.UniqueReporters,
		"total_reports":              aggregated.TotalReports,
		"is_official_blacklisted":    aggregated.IsOfficialBlacklisted,
		"is_whitelisted_verified":    aggregated.IsWhitelistedVerified,
		"sync_version":               aggregated.SyncVersion,
		"first_reported_at":          aggregated.FirstReportedAt,
		"last_reported_at":           aggregated.LastReportedAt,
		"last_activity_at":           aggregated.LastActivityAt,
		"expires_at":                 aggregated.ExpiresAt,
		"updated_at":                 aggregated.UpdatedAt,
	}
}

func recomputePhoneTagMongo(ctx context.Context, repCol, ptCol *mongo.Collection, phoneE164 string) (model.PhoneTag, error) {
	cur, err := repCol.Find(ctx, bson.M{"phone_e164": phoneE164})
	if err != nil {
		return model.PhoneTag{}, err
	}
	var allReports []model.SpamReport
	if err := cur.All(ctx, &allReports); err != nil {
		_ = cur.Close(ctx)
		return model.PhoneTag{}, err
	}
	_ = cur.Close(ctx)

	var existing model.PhoneTag
	var existingPtr *model.PhoneTag
	if err := ptCol.FindOne(ctx, bson.M{"phone_e164": phoneE164}).Decode(&existing); err == nil {
		existingPtr = &existing
	}

	nextSync := time.Now().Unix()
	if existingPtr != nil && nextSync <= existingPtr.SyncVersion {
		nextSync = existingPtr.SyncVersion + 1
	}
	aggregated := helper.AggregatePhoneTag(existingPtr, allReports, nextSync)

	upsertOpts := options.Update().SetUpsert(true)
	_, err = ptCol.UpdateOne(
		ctx,
		bson.M{"phone_e164": phoneE164},
		bson.M{"$set": phoneTagBsonSet(aggregated)},
		upsertOpts,
	)
	if err != nil {
		return model.PhoneTag{}, err
	}

	op := "UPSERT"
	if aggregated.TotalReports == 0 && aggregated.RiskScore == 0 && !aggregated.IsOfficialBlacklisted {
		op = "DELETE"
	}
	if _, err := appendSyncChangeMongo(ctx, op, "PHONE_TAG", aggregated.PhoneE164, buildPhoneTagSyncPayload(aggregated)); err != nil {
		return model.PhoneTag{}, err
	}
	if aggregated.PendingDangerReview {
		ensurePendingDangerReviewCaseMongo(ctx, aggregated.PhoneE164, aggregated.UpdatedAt)
	}
	return aggregated, nil
}

func getCurrentPhoneTagMongo(ctx context.Context, repCol, ptCol *mongo.Collection, phoneE164 string) (model.PhoneTag, error) {
	var current model.PhoneTag
	if err := ptCol.FindOne(ctx, bson.M{"phone_e164": phoneE164}).Decode(&current); err == nil {
		current.SpamScore = current.RiskScore
		return current, nil
	}
	return recomputePhoneTagMongo(ctx, repCol, ptCol, phoneE164)
}

// CreateReportAndAggregate membungkus CreateReportWithIdempotency untuk kompatibilitas pemanggil lama.
func CreateReportAndAggregate(ctx context.Context, report model.SpamReport) (model.PhoneTag, error) {
	_, tag, _, err := CreateReportWithIdempotency(ctx, report, report.IdempotencyKey)
	return tag, err
}

// CreateReportWithIdempotency menyimpan laporan secara idempoten (T-104), menerapkan kuota pelapor (T-103),
// melakukan deduplikasi 30 hari, serta mengagregasi ulang skor di PhoneTags.
// Nilai balik bool (created) bernilai true jika dokumen baru disimpan, dan false jika merupakan replay/duplikat dalam 30 hari.
func CreateReportWithIdempotency(ctx context.Context, report model.SpamReport, idempotencyKey string) (*model.SpamReport, model.PhoneTag, bool, error) {
	now := time.Now().UTC()
	const windowSeconds int64 = 30 * 24 * 3600
	windowKey := now.Unix() / windowSeconds
	if windowKey <= 0 {
		windowKey = 1
	}
	cutoff30d := now.Add(-30 * 24 * time.Hour)
	cutoff5m := now.Add(-5 * time.Minute)
	cutoff24h := now.Add(-24 * time.Hour)

	idempotencyKey = strings.TrimSpace(idempotencyKey)
	report.ID = primitive.NewObjectID()
	report.IdempotencyKey = idempotencyKey
	report.WindowKey = windowKey
	report.Status = "VALIDATED"
	report.CreatedAt = now

	if MongoConnected && MongoDB != nil {
		repCol := MongoDB.Collection(ColSpamReports)
		ptCol := MongoDB.Collection(ColPhoneTags)

		// 1. Cek idempotency_key terlebih dahulu (sebelum memotong kuota)
		if idempotencyKey != "" {
			var existingRep model.SpamReport
			err := repCol.FindOne(ctx, bson.M{
				"reporter_user_id": report.ReporterUserID,
				"idempotency_key":  idempotencyKey,
			}).Decode(&existingRep)
			if err == nil {
				if existingRep.PhoneE164 != report.PhoneE164 {
					return nil, model.PhoneTag{}, false, ErrIdempotencyMismatch
				}
				tag, err := getCurrentPhoneTagMongo(ctx, repCol, ptCol, existingRep.PhoneE164)
				return &existingRep, tag, false, err
			}
		}

		// 2. Cek deduplikasi 30 hari per (phone_e164, reporter_user_id)
		var dupRep model.SpamReport
		err := repCol.FindOne(ctx, bson.M{
			"phone_e164":       report.PhoneE164,
			"reporter_user_id": report.ReporterUserID,
			"created_at":       bson.M{"$gte": cutoff30d},
		}).Decode(&dupRep)
		if err == nil {
			tag, err := getCurrentPhoneTagMongo(ctx, repCol, ptCol, dupRep.PhoneE164)
			return &dupRep, tag, false, err
		}

		// 3. Terapkan batas kuota pelapor per akun JWT (maks 3/5 menit dan 10/24 jam)
		count5m, err := repCol.CountDocuments(ctx, bson.M{
			"reporter_user_id": report.ReporterUserID,
			"created_at":       bson.M{"$gte": cutoff5m},
		})
		if err != nil {
			return nil, model.PhoneTag{}, false, err
		}
		if count5m >= 3 {
			return nil, model.PhoneTag{}, false, ErrQuotaExceeded
		}

		count24h, err := repCol.CountDocuments(ctx, bson.M{
			"reporter_user_id": report.ReporterUserID,
			"created_at":       bson.M{"$gte": cutoff24h},
		})
		if err != nil {
			return nil, model.PhoneTag{}, false, err
		}
		if count24h >= 10 {
			return nil, model.PhoneTag{}, false, ErrQuotaExceeded
		}

		// 4. Simpan dokumen baru (tangani race duplicate key secara idempoten)
		if _, err := repCol.InsertOne(ctx, report); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				var raced model.SpamReport
				if idempotencyKey != "" {
					if findErr := repCol.FindOne(ctx, bson.M{
						"reporter_user_id": report.ReporterUserID,
						"idempotency_key":  idempotencyKey,
					}).Decode(&raced); findErr == nil {
						tag, tagErr := getCurrentPhoneTagMongo(ctx, repCol, ptCol, raced.PhoneE164)
						return &raced, tag, false, tagErr
					}
				}
				if findErr := repCol.FindOne(ctx, bson.M{
					"phone_e164":       report.PhoneE164,
					"reporter_user_id": report.ReporterUserID,
					"window_key":       windowKey,
				}).Decode(&raced); findErr == nil {
					tag, tagErr := getCurrentPhoneTagMongo(ctx, repCol, ptCol, raced.PhoneE164)
					return &raced, tag, false, tagErr
				}
			}
			return nil, model.PhoneTag{}, false, err
		}

		aggregated, err := recomputePhoneTagMongo(ctx, repCol, ptCol, report.PhoneE164)
		if err != nil {
			return nil, model.PhoneTag{}, false, err
		}
		return &report, aggregated, true, nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	// 1. Cek idempotency_key di memori
	if idempotencyKey != "" {
		for _, list := range memSpamReports {
			for _, r := range list {
				if r.ReporterUserID == report.ReporterUserID && r.IdempotencyKey == idempotencyKey {
					if r.PhoneE164 != report.PhoneE164 {
						return nil, model.PhoneTag{}, false, ErrIdempotencyMismatch
					}
					existingCopy := r
					tag := memPhoneTags[r.PhoneE164]
					return &existingCopy, tag, false, nil
				}
			}
		}
	}

	// 2. Cek deduplikasi 30 hari di memori
	for _, r := range memSpamReports[report.PhoneE164] {
		if r.ReporterUserID == report.ReporterUserID && !r.CreatedAt.Before(cutoff30d) {
			existingCopy := r
			tag := memPhoneTags[report.PhoneE164]
			return &existingCopy, tag, false, nil
		}
	}

	// 3. Cek kuota pelapor di memori (3 per 5 menit & 10 per 24 jam)
	var count5m, count24h int
	for _, list := range memSpamReports {
		for _, r := range list {
			if r.ReporterUserID != report.ReporterUserID {
				continue
			}
			if !r.CreatedAt.Before(cutoff5m) {
				count5m++
			}
			if !r.CreatedAt.Before(cutoff24h) {
				count24h++
			}
		}
	}
	if count5m >= 3 || count24h >= 10 {
		return nil, model.PhoneTag{}, false, ErrQuotaExceeded
	}

	// 4. Simpan dan agregasi ulang
	existing, hasExisting := memPhoneTags[report.PhoneE164]
	var existingPtr *model.PhoneTag
	if hasExisting {
		existingPtr = &existing
	}

	allReports := append(memSpamReports[report.PhoneE164], report)
	memSpamReports[report.PhoneE164] = allReports

	memSyncCounter++
	aggregated := helper.AggregatePhoneTag(existingPtr, allReports, memSyncCounter)
	if aggregated.ID.IsZero() {
		aggregated.ID = primitive.NewObjectID()
	}
	memPhoneTags[report.PhoneE164] = aggregated
	appendSyncChangeMemoryLocked("UPSERT", "PHONE_TAG", aggregated.PhoneE164, buildPhoneTagSyncPayload(aggregated))
	if aggregated.PendingDangerReview {
		ensurePendingDangerReviewCaseMemLocked(aggregated.PhoneE164, aggregated.UpdatedAt)
	}
	return &report, aggregated, true, nil
}

// CreateDisputeCase menyimpan pengajuan sanggahan secara persisten ke moderation_cases sebelum respons 201 (T-105).
func CreateDisputeCase(ctx context.Context, req model.DisputeRequest, submitterID string) (*model.ModerationCase, string, error) {
	phoneE164 := helper.NormalizePhoneID(req.Phone)
	now := time.Now().UTC()
	cutoff24h := now.Add(-24 * time.Hour)

	objID := primitive.NewObjectID()
	disputeID := "DSP_" + objID.Hex()
	trackingToken := helper.HashValue(disputeID + ":" + submitterID + ":" + now.Format(time.RFC3339Nano))[:32]
	trackingTokenHash := helper.HashValue("tracking:" + trackingToken)

	doc := model.ModerationCase{
		ID:                objID,
		DisputeID:         disputeID,
		Type:              "DISPUTE",
		PhoneE164:         phoneE164,
		OwnerName:         strings.TrimSpace(req.OwnerName),
		Reason:            strings.TrimSpace(req.Reason),
		ContactRef:        strings.TrimSpace(req.ContactRef),
		SubmitterID:       submitterID,
		Status:            "UNDER_REVIEW",
		TrackingTokenHash: trackingTokenHash,
		Version:           1,
		SubmittedAt:       now,
		UpdatedAt:         now,
	}

	if MongoConnected && MongoDB != nil {
		col := MongoDB.Collection(ColModerationCases)

		activeCount, err := col.CountDocuments(ctx, bson.M{
			"phone_e164":   phoneE164,
			"submitter_id": submitterID,
			"status":       bson.M{"$in": []string{"OPEN", "UNDER_REVIEW", "PROCESSING", "NEEDS_INFO"}},
		})
		if err != nil {
			return nil, "", err
		}
		if activeCount > 0 {
			return nil, "", ErrDuplicateDispute
		}

		recentCount, err := col.CountDocuments(ctx, bson.M{
			"submitter_id": submitterID,
			"submitted_at": bson.M{"$gte": cutoff24h},
		})
		if err != nil {
			return nil, "", err
		}
		if recentCount >= 3 {
			return nil, "", ErrQuotaExceeded
		}

		if _, err := col.InsertOne(ctx, doc); err != nil {
			return nil, "", err
		}
		return &doc, trackingToken, nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	recentCount := 0
	for _, c := range memModerationCases {
		if c.PhoneE164 == phoneE164 && c.SubmitterID == submitterID && (c.Status == "OPEN" || c.Status == "UNDER_REVIEW" || c.Status == "PROCESSING" || c.Status == "NEEDS_INFO") {
			return nil, "", ErrDuplicateDispute
		}
		if c.SubmitterID == submitterID && !c.SubmittedAt.Before(cutoff24h) {
			recentCount++
		}
	}
	if recentCount >= 3 {
		return nil, "", ErrQuotaExceeded
	}

	memModerationCases[disputeID] = doc
	return &doc, trackingToken, nil
}

// GetDisputeCaseByID mengambil data kasus sanggahan dari moderation_cases berdasarkan dispute_id (T-105).
func GetDisputeCaseByID(ctx context.Context, disputeID string) (*model.ModerationCase, error) {
	disputeID = strings.TrimSpace(disputeID)
	if disputeID == "" {
		return nil, ErrNotFound
	}

	if MongoConnected && MongoDB != nil {
		var doc model.ModerationCase
		err := MongoDB.Collection(ColModerationCases).FindOne(ctx, bson.M{"dispute_id": disputeID}).Decode(&doc)
		if err != nil {
			if err == mongo.ErrNoDocuments {
				return nil, ErrNotFound
			}
			return nil, err
		}
		return &doc, nil
	}

	memMu.RLock()
	defer memMu.RUnlock()

	doc, ok := memModerationCases[disputeID]
	if !ok {
		return nil, ErrNotFound
	}
	return &doc, nil
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
		for _, ch := range memOfficialChannels {
			if ch.IsActive {
				all = append(all, ch)
			}
		}
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

func buildPhoneTagSyncPayload(pt model.PhoneTag) map[string]interface{} {
	score := pt.RiskScore
	if score == 0 {
		score = pt.SpamScore
	}
	return map[string]interface{}{
		"phone_e164":             pt.PhoneE164,
		"reputation_level":       string(pt.ReputationLevel),
		"risk_score":             score,
		"is_danger":              pt.IsDanger,
		"pending_danger_review":  pt.PendingDangerReview,
		"primary_category":       string(pt.PrimaryCategory),
		"primary_category_label": pt.PrimaryCategoryLabel,
		"top_community_label":    pt.TopCommunityLabel,
		"unique_reporters":       pt.UniqueReporters,
		"total_reports":          pt.TotalReports,
		"sync_version":           pt.SyncVersion,
	}
}

func appendSyncChangeMemoryLocked(op, kind, key string, payload map[string]interface{}) model.SyncChange {
	memSyncSeq++
	entry := model.SyncChange{
		ID:         primitive.NewObjectID(),
		Seq:        memSyncSeq,
		Generation: memSyncGen,
		Op:         op,
		Kind:       kind,
		Key:        key,
		Payload:    payload,
		At:         time.Now().UTC(),
	}
	memSyncChanges = append(memSyncChanges, entry)
	return entry
}

func nextSyncSeqAndGenMongo(ctx context.Context) (int64, int, error) {
	col := MongoDB.Collection(ColSyncCounters)
	opts := options.FindOneAndUpdate().SetUpsert(true).SetReturnDocument(options.After)
	var doc struct {
		Seq        int64 `bson:"seq"`
		Generation int   `bson:"generation"`
	}
	err := col.FindOneAndUpdate(
		ctx,
		bson.M{"_id": "sync_changes"},
		bson.M{
			"$inc":         bson.M{"seq": int64(1)},
			"$setOnInsert": bson.M{"generation": 1},
		},
		opts,
	).Decode(&doc)
	if err != nil {
		return 0, 1, err
	}
	if doc.Generation <= 0 {
		doc.Generation = 1
	}
	return doc.Seq, doc.Generation, nil
}

func getCurrentSyncGenMongo(ctx context.Context) (int, error) {
	var doc struct {
		Generation int `bson:"generation"`
	}
	err := MongoDB.Collection(ColSyncCounters).FindOne(ctx, bson.M{"_id": "sync_changes"}).Decode(&doc)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return 1, nil
		}
		return 1, err
	}
	if doc.Generation <= 0 {
		return 1, nil
	}
	return doc.Generation, nil
}

func appendSyncChangeMongo(ctx context.Context, op, kind, key string, payload map[string]interface{}) (model.SyncChange, error) {
	seq, gen, err := nextSyncSeqAndGenMongo(ctx)
	if err != nil {
		return model.SyncChange{}, err
	}
	entry := model.SyncChange{
		ID:         primitive.NewObjectID(),
		Seq:        seq,
		Generation: gen,
		Op:         op,
		Kind:       kind,
		Key:        key,
		Payload:    payload,
		At:         time.Now().UTC(),
	}
	if _, err := MongoDB.Collection(ColSyncChanges).InsertOne(ctx, entry); err != nil {
		return model.SyncChange{}, err
	}
	return entry, nil
}

func ensurePendingDangerReviewCaseMemLocked(phoneE164 string, now time.Time) {
	for _, c := range memModerationCases {
		if c.Type == "REPORT_REVIEW" && c.PhoneE164 == phoneE164 && (c.Status == "OPEN" || c.Status == "UNDER_REVIEW" || c.Status == "PROCESSING" || c.Status == "NEEDS_INFO") {
			return
		}
	}
	objID := primitive.NewObjectID()
	caseID := "REV_" + objID.Hex()
	memModerationCases[caseID] = model.ModerationCase{
		ID:          objID,
		DisputeID:   caseID,
		Type:        "REPORT_REVIEW",
		PhoneE164:   phoneE164,
		OwnerName:   "Kandidat DANGER_SCAM",
		Reason:      "Memenuhi ambang skor >= 70 dan N >= 5; menunggu verifikasi moderator (OD-03)",
		SubmitterID: "system:scoring_gate",
		Status:      "OPEN",
		Version:     1,
		SubmittedAt: now,
		UpdatedAt:   now,
	}
}

func ensurePendingDangerReviewCaseMongo(ctx context.Context, phoneE164 string, now time.Time) {
	col := MongoDB.Collection(ColModerationCases)
	cnt, err := col.CountDocuments(ctx, bson.M{
		"type":       "REPORT_REVIEW",
		"phone_e164": phoneE164,
		"status":     bson.M{"$in": []string{"OPEN", "UNDER_REVIEW", "PROCESSING", "NEEDS_INFO"}},
	})
	if err != nil || cnt > 0 {
		return
	}
	objID := primitive.NewObjectID()
	caseID := "REV_" + objID.Hex()
	_, _ = col.InsertOne(ctx, model.ModerationCase{
		ID:          objID,
		DisputeID:   caseID,
		Type:        "REPORT_REVIEW",
		PhoneE164:   phoneE164,
		OwnerName:   "Kandidat DANGER_SCAM",
		Reason:      "Memenuhi ambang skor >= 70 dan N >= 5; menunggu verifikasi moderator (OD-03)",
		SubmitterID: "system:scoring_gate",
		Status:      "OPEN",
		Version:     1,
		SubmittedAt: now,
		UpdatedAt:   now,
	})
}

func parseSyncCursor(cursor string, currentGen int) (int64, error) {
	trimmed := strings.TrimSpace(cursor)
	if trimmed == "" {
		return 0, nil
	}
	parts := strings.Split(trimmed, ":")
	if len(parts) != 2 {
		return 0, ErrInvalidCursor
	}
	gen, err := strconv.Atoi(parts[0])
	if err != nil || gen <= 0 {
		return 0, ErrInvalidCursor
	}
	seq, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || seq < 0 {
		return 0, ErrInvalidCursor
	}
	if gen != currentGen {
		return 0, ErrFullResyncRequired
	}
	return seq, nil
}

// GetSyncChanges membaca jurnal perubahan monoton sync_changes dengan paginasi cursor generasi:seq (T-201).
func GetSyncChanges(ctx context.Context, cursor string, limit int) (model.SyncChangesResponse, error) {
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}

	if MongoConnected && MongoDB != nil {
		gen, err := getCurrentSyncGenMongo(ctx)
		if err != nil {
			return model.SyncChangesResponse{}, err
		}
		afterSeq, err := parseSyncCursor(cursor, gen)
		if err != nil {
			return model.SyncChangesResponse{}, err
		}

		opts := options.Find().SetSort(bson.D{{Key: "seq", Value: 1}}).SetLimit(int64(limit + 1))
		cur, err := MongoDB.Collection(ColSyncChanges).Find(ctx, bson.M{
			"generation": gen,
			"seq":        bson.M{"$gt": afterSeq},
		}, opts)
		if err != nil {
			return model.SyncChangesResponse{}, err
		}
		var fetched []model.SyncChange
		if err := cur.All(ctx, &fetched); err != nil {
			_ = cur.Close(ctx)
			return model.SyncChangesResponse{}, err
		}
		_ = cur.Close(ctx)

		hasMore := len(fetched) > limit
		if hasMore {
			fetched = fetched[:limit]
		}
		if fetched == nil {
			fetched = []model.SyncChange{}
		}
		nextSeq := afterSeq
		if len(fetched) > 0 {
			nextSeq = fetched[len(fetched)-1].Seq
		}
		return model.SyncChangesResponse{
			Generation: gen,
			Count:      len(fetched),
			Items:      fetched,
			NextCursor: fmt.Sprintf("%d:%d", gen, nextSeq),
			HasMore:    hasMore,
			SyncedAt:   time.Now().UTC(),
		}, nil
	}

	memMu.RLock()
	defer memMu.RUnlock()

	gen := memSyncGen
	afterSeq, err := parseSyncCursor(cursor, gen)
	if err != nil {
		return model.SyncChangesResponse{}, err
	}

	var matched []model.SyncChange
	for _, item := range memSyncChanges {
		if item.Generation == gen && item.Seq > afterSeq {
			matched = append(matched, item)
			if len(matched) > limit {
				break
			}
		}
	}

	hasMore := len(matched) > limit
	if hasMore {
		matched = matched[:limit]
	}
	if matched == nil {
		matched = []model.SyncChange{}
	}
	nextSeq := afterSeq
	if len(matched) > 0 {
		nextSeq = matched[len(matched)-1].Seq
	}
	return model.SyncChangesResponse{
		Generation: gen,
		Count:      len(matched),
		Items:      matched,
		NextCursor: fmt.Sprintf("%d:%d", gen, nextSeq),
		HasMore:    hasMore,
		SyncedAt:   time.Now().UTC(),
	}, nil
}

func generateOpaqueToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

// IssueRefreshToken menerbitkan refresh token acak baru (hanya menyimpan hash di penyimpanan) untuk T-204.
func IssueRefreshToken(ctx context.Context, userID string) (string, error) {
	raw, err := generateOpaqueToken()
	if err != nil {
		return "", err
	}
	now := time.Now().UTC()
	doc := model.RefreshTokenDoc{
		ID:        primitive.NewObjectID(),
		TokenHash: helper.HashValue("refresh:" + raw),
		UserID:    userID,
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		CreatedAt: now,
	}

	if MongoConnected && MongoDB != nil {
		if _, err := MongoDB.Collection(ColRefreshTokens).InsertOne(ctx, doc); err != nil {
			return "", err
		}
		return raw, nil
	}

	memMu.Lock()
	memRefreshTokens[doc.TokenHash] = doc
	memMu.Unlock()
	return raw, nil
}

// RotateRefreshToken merotasi refresh token dan mendeteksi replay reuse token yang telah dicabut (T-204).
func RotateRefreshToken(ctx context.Context, rawToken string) (model.User, string, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	tokenHash := helper.HashValue("refresh:" + rawToken)
	now := time.Now().UTC()

	if MongoConnected && MongoDB != nil {
		rtCol := MongoDB.Collection(ColRefreshTokens)
		uCol := MongoDB.Collection(ColUsers)

		var doc model.RefreshTokenDoc
		if err := rtCol.FindOne(ctx, bson.M{"token_hash": tokenHash}).Decode(&doc); err != nil {
			return model.User{}, "", ErrInvalidRefreshToken
		}

		// Reuse detection: token sudah dicabut sebelumnya -> cabut seluruh sesi pengguna
		if doc.RevokedAt != nil {
			_, _ = rtCol.UpdateMany(ctx, bson.M{"user_id": doc.UserID, "revoked_at": nil}, bson.M{"$set": bson.M{"revoked_at": now}})
			if objID, convErr := primitive.ObjectIDFromHex(doc.UserID); convErr == nil {
				_, _ = uCol.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$inc": bson.M{"token_version": 1}})
			}
			return model.User{}, "", ErrInvalidRefreshToken
		}

		if now.After(doc.ExpiresAt) {
			_, _ = rtCol.UpdateOne(ctx, bson.M{"_id": doc.ID}, bson.M{"$set": bson.M{"revoked_at": now}})
			return model.User{}, "", ErrInvalidRefreshToken
		}

		res, err := rtCol.UpdateOne(
			ctx,
			bson.M{"_id": doc.ID, "revoked_at": nil},
			bson.M{"$set": bson.M{"revoked_at": now}},
		)
		if err != nil {
			return model.User{}, "", err
		}
		if res.MatchedCount == 0 {
			_, _ = rtCol.UpdateMany(ctx, bson.M{"user_id": doc.UserID, "revoked_at": nil}, bson.M{"$set": bson.M{"revoked_at": now}})
			return model.User{}, "", ErrInvalidRefreshToken
		}

		user, err := GetUserByID(ctx, doc.UserID)
		if err != nil {
			return model.User{}, "", ErrInvalidRefreshToken
		}
		if strings.EqualFold(user.Status, "DELETION_REQUESTED") || strings.EqualFold(user.Status, "DELETED") {
			return model.User{}, "", ErrUserInactive
		}

		newRaw, err := IssueRefreshToken(ctx, doc.UserID)
		if err != nil {
			return model.User{}, "", err
		}
		return user, newRaw, nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	doc, ok := memRefreshTokens[tokenHash]
	if !ok {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if doc.RevokedAt != nil {
		for h, item := range memRefreshTokens {
			if item.UserID == doc.UserID && item.RevokedAt == nil {
				tCopy := now
				item.RevokedAt = &tCopy
				memRefreshTokens[h] = item
			}
		}
		for k, u := range memUsers {
			if u.ID.Hex() == doc.UserID {
				u.TokenVersion++
				memUsers[k] = u
			}
		}
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if now.After(doc.ExpiresAt) {
		tCopy := now
		doc.RevokedAt = &tCopy
		memRefreshTokens[tokenHash] = doc
		return model.User{}, "", ErrInvalidRefreshToken
	}

	tCopy := now
	doc.RevokedAt = &tCopy
	memRefreshTokens[tokenHash] = doc

	var foundUser model.User
	found := false
	for _, u := range memUsers {
		if u.ID.Hex() == doc.UserID {
			foundUser = u
			found = true
			break
		}
	}
	if !found {
		return model.User{}, "", ErrInvalidRefreshToken
	}
	if strings.EqualFold(foundUser.Status, "DELETION_REQUESTED") || strings.EqualFold(foundUser.Status, "DELETED") {
		return model.User{}, "", ErrUserInactive
	}
	if foundUser.TokenVersion <= 0 {
		foundUser.TokenVersion = 1
	}

	newRaw, err := generateOpaqueToken()
	if err != nil {
		return model.User{}, "", err
	}
	newHash := helper.HashValue("refresh:" + newRaw)
	memRefreshTokens[newHash] = model.RefreshTokenDoc{
		ID:        primitive.NewObjectID(),
		TokenHash: newHash,
		UserID:    foundUser.ID.Hex(),
		ExpiresAt: now.Add(30 * 24 * time.Hour),
		CreatedAt: now,
	}
	return foundUser, newRaw, nil
}

// GetUserByID mengambil dokumen pengguna berdasarkan ID hex string.
func GetUserByID(ctx context.Context, userID string) (model.User, error) {
	if MongoConnected && MongoDB != nil {
		objID, err := primitive.ObjectIDFromHex(userID)
		if err != nil {
			return model.User{}, ErrNotFound
		}
		var u model.User
		if err := MongoDB.Collection(ColUsers).FindOne(ctx, bson.M{"_id": objID}).Decode(&u); err != nil {
			if err == mongo.ErrNoDocuments {
				return model.User{}, ErrNotFound
			}
			return model.User{}, err
		}
		if u.TokenVersion <= 0 {
			u.TokenVersion = 1
		}
		return u, nil
	}

	memMu.RLock()
	defer memMu.RUnlock()

	for _, u := range memUsers {
		if u.ID.Hex() == userID {
			if u.TokenVersion <= 0 {
				u.TokenVersion = 1
			}
			return u, nil
		}
	}
	return model.User{}, ErrNotFound
}

// IsUserActive memverifikasi status akun belum dihapus dan token_version belum dicabut.
func IsUserActive(ctx context.Context, userID string, tokenVersion int) bool {
	u, err := GetUserByID(ctx, userID)
	if err != nil {
		return false
	}
	if strings.EqualFold(u.Status, "DELETION_REQUESTED") || strings.EqualFold(u.Status, "DELETED") || strings.EqualFold(u.Status, "BANNED") {
		return false
	}
	if tokenVersion > 0 && u.TokenVersion > tokenVersion {
		return false
	}
	return true
}

// GetUserQuotaRemaining menghitung sisa kuota laporan (5m, 24h) dan sanggahan (24h) pengguna (T-204).
func GetUserQuotaRemaining(ctx context.Context, userID string) (model.QuotaRemaining, error) {
	now := time.Now().UTC()
	cutoff5m := now.Add(-5 * time.Minute)
	cutoff24h := now.Add(-24 * time.Hour)

	var c5m, c24h, d24h int
	if MongoConnected && MongoDB != nil {
		repCol := MongoDB.Collection(ColSpamReports)
		modCol := MongoDB.Collection(ColModerationCases)

		n5m, err := repCol.CountDocuments(ctx, bson.M{
			"reporter_user_id": userID,
			"created_at":       bson.M{"$gte": cutoff5m},
		})
		if err != nil {
			return model.QuotaRemaining{}, err
		}
		n24h, err := repCol.CountDocuments(ctx, bson.M{
			"reporter_user_id": userID,
			"created_at":       bson.M{"$gte": cutoff24h},
		})
		if err != nil {
			return model.QuotaRemaining{}, err
		}
		nd24h, err := modCol.CountDocuments(ctx, bson.M{
			"submitter_id": "user:" + userID,
			"submitted_at": bson.M{"$gte": cutoff24h},
		})
		if err != nil {
			return model.QuotaRemaining{}, err
		}
		c5m, c24h, d24h = int(n5m), int(n24h), int(nd24h)
	} else {
		memMu.RLock()
		for _, list := range memSpamReports {
			for _, r := range list {
				if r.ReporterUserID != userID {
					continue
				}
				if !r.CreatedAt.Before(cutoff5m) {
					c5m++
				}
				if !r.CreatedAt.Before(cutoff24h) {
					c24h++
				}
			}
		}
		for _, c := range memModerationCases {
			if c.SubmitterID == "user:"+userID && !c.SubmittedAt.Before(cutoff24h) {
				d24h++
			}
		}
		memMu.RUnlock()
	}

	rem5m := 3 - c5m
	if rem5m < 0 {
		rem5m = 0
	}
	rem24h := 10 - c24h
	if rem24h < 0 {
		rem24h = 0
	}
	remDisp := 3 - d24h
	if remDisp < 0 {
		remDisp = 0
	}
	return model.QuotaRemaining{
		Reports5mRemaining:   rem5m,
		Reports24hRemaining:  rem24h,
		Disputes24hRemaining: remDisp,
	}, nil
}

// RequestAccountDeletion menandai akun terhapus (UU PDP), mencabut sesi, dan menganonimkan laporan tanpa menghapus agregat (K-12, T-204).
func RequestAccountDeletion(ctx context.Context, userID string) error {
	now := time.Now().UTC()
	anonID := "anon_" + helper.HashValue("anon:" + userID)[:16]

	if MongoConnected && MongoDB != nil {
		objID, err := primitive.ObjectIDFromHex(userID)
		if err != nil {
			return ErrNotFound
		}
		uCol := MongoDB.Collection(ColUsers)
		var u model.User
		if err := uCol.FindOne(ctx, bson.M{"_id": objID}).Decode(&u); err != nil {
			if err == mongo.ErrNoDocuments {
				return ErrNotFound
			}
			return err
		}
		if strings.EqualFold(u.Status, "DELETION_REQUESTED") || strings.EqualFold(u.Status, "DELETED") {
			return ErrUserInactive
		}

		if _, err := uCol.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{
			"$set": bson.M{
				"status":       "DELETION_REQUESTED",
				"display_name": "Akun Dihapus",
				"phone_mask":   "",
				"updated_at":   now,
			},
			"$unset": bson.M{
				"phone_hash":     "",
				"device_id_hash": "",
				"google_sub":     "",
				"email":          "",
			},
			"$inc": bson.M{"token_version": 1},
		}); err != nil {
			return err
		}

		_, _ = MongoDB.Collection(ColRefreshTokens).UpdateMany(
			ctx,
			bson.M{"user_id": userID, "revoked_at": nil},
			bson.M{"$set": bson.M{"revoked_at": now}},
		)
		_, _ = MongoDB.Collection(ColSpamReports).UpdateMany(
			ctx,
			bson.M{"reporter_user_id": userID},
			bson.M{"$set": bson.M{"reporter_user_id": anonID, "idempotency_key": ""}},
		)
		return nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	foundKey := ""
	var foundUser model.User
	for k, u := range memUsers {
		if u.ID.Hex() == userID {
			foundKey = k
			foundUser = u
			break
		}
	}
	if foundKey == "" {
		return ErrNotFound
	}
	if strings.EqualFold(foundUser.Status, "DELETION_REQUESTED") || strings.EqualFold(foundUser.Status, "DELETED") {
		return ErrUserInactive
	}

	foundUser.Status = "DELETION_REQUESTED"
	foundUser.DisplayName = "Akun Dihapus"
	foundUser.PhoneMask = ""
	foundUser.PhoneHash = ""
	foundUser.DeviceIDHash = ""
	foundUser.TokenVersion++
	foundUser.UpdatedAt = now
	delete(memUsers, foundKey)
	memUsers["deleted:"+foundUser.ID.Hex()] = foundUser

	for h, rt := range memRefreshTokens {
		if rt.UserID == userID && rt.RevokedAt == nil {
			tCopy := now
			rt.RevokedAt = &tCopy
			memRefreshTokens[h] = rt
		}
	}

	for phone, list := range memSpamReports {
		changed := false
		for i := range list {
			if list[i].ReporterUserID == userID {
				list[i].ReporterUserID = anonID
				list[i].IdempotencyKey = ""
				changed = true
			}
		}
		if changed {
			memSpamReports[phone] = list
		}
	}

	return nil
}

func phoneTagNeedsReconcile(prev, next model.PhoneTag) bool {
	return prev.ReputationLevel != next.ReputationLevel ||
		prev.RiskScore != next.RiskScore ||
		prev.TotalReports != next.TotalReports ||
		prev.UniqueReporters != next.UniqueReporters ||
		prev.PendingDangerReview != next.PendingDangerReview ||
		prev.PrimaryCategory != next.PrimaryCategory
}

// ReconcileAllPhoneTags merekonsiliasi ulang seluruh agregat phone_tags terhadap waktu now secara idempoten (T-205, T-206).
func ReconcileAllPhoneTags(ctx context.Context, now time.Time) (int, error) {
	now = now.UTC()
	updated := 0

	if MongoConnected && MongoDB != nil {
		ptCol := MongoDB.Collection(ColPhoneTags)
		repCol := MongoDB.Collection(ColSpamReports)

		cur, err := ptCol.Find(ctx, bson.M{})
		if err != nil {
			return 0, err
		}
		var allTags []model.PhoneTag
		if err := cur.All(ctx, &allTags); err != nil {
			_ = cur.Close(ctx)
			return 0, err
		}
		_ = cur.Close(ctx)

		for _, existing := range allTags {
			rc, err := repCol.Find(ctx, bson.M{"phone_e164": existing.PhoneE164})
			if err != nil {
				return updated, err
			}
			var reports []model.SpamReport
			if err := rc.All(ctx, &reports); err != nil {
				_ = rc.Close(ctx)
				return updated, err
			}
			_ = rc.Close(ctx)

			// Seed awal tanpa dokumen spam_reports dipertahankan hingga > 365 hari sejak aktivitas terakhir.
			if len(reports) == 0 && existing.TotalReports > 0 && !existing.LastReportedAt.IsZero() &&
				now.Sub(existing.LastReportedAt).Hours()/24.0 <= helper.ReportMaxAgeDays {
				continue
			}

			nextSync := existing.SyncVersion + 1
			existingCopy := existing
			aggregated := helper.AggregatePhoneTagWithTime(&existingCopy, reports, nextSync, now)
			if !phoneTagNeedsReconcile(existing, aggregated) {
				continue
			}

			if _, err := ptCol.UpdateOne(ctx, bson.M{"phone_e164": existing.PhoneE164}, bson.M{"$set": phoneTagBsonSet(aggregated)}); err != nil {
				return updated, err
			}
			op := "UPSERT"
			if aggregated.TotalReports == 0 && aggregated.RiskScore == 0 && !aggregated.IsOfficialBlacklisted {
				op = "DELETE"
			}
			_, _ = appendSyncChangeMongo(ctx, op, "PHONE_TAG", aggregated.PhoneE164, buildPhoneTagSyncPayload(aggregated))
			if aggregated.PendingDangerReview {
				ensurePendingDangerReviewCaseMongo(ctx, aggregated.PhoneE164, now)
			}
			updated++
		}
		return updated, nil
	}

	memMu.Lock()
	defer memMu.Unlock()

	for phone, existing := range memPhoneTags {
		reports := memSpamReports[phone]
		memSyncCounter++
		existingCopy := existing
		aggregated := helper.AggregatePhoneTagWithTime(&existingCopy, reports, memSyncCounter, now)
		if !phoneTagNeedsReconcile(existing, aggregated) {
			continue
		}
		memPhoneTags[phone] = aggregated
		op := "UPSERT"
		if aggregated.TotalReports == 0 && aggregated.RiskScore == 0 && !aggregated.IsOfficialBlacklisted {
			op = "DELETE"
		}
		appendSyncChangeMemoryLocked(op, "PHONE_TAG", aggregated.PhoneE164, buildPhoneTagSyncPayload(aggregated))
		if aggregated.PendingDangerReview {
			ensurePendingDangerReviewCaseMemLocked(aggregated.PhoneE164, now)
		}
		updated++
	}
	return updated, nil
}
