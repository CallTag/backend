package config

import (
	"context"
	"regexp"
	"sort"
	"strings"

	"github.com/CallTag/backend/helper"
	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type AdminNumberRow struct {
	Phone       string                `json:"phone_e164" bson:"phone_e164"`
	Reputation  model.ReputationLevel `json:"reputation_level" bson:"reputation_level"`
	Category    string                `json:"primary_category_label" bson:"primary_category_label"`
	Label       string                `json:"top_community_label" bson:"top_community_label"`
	Tags        []model.TagCount      `json:"tags" bson:"tags"`
	ReportCount int                   `json:"total_reports" bson:"total_reports"`
}

type AdminNumberPage struct {
	Items []AdminNumberRow `json:"items"`
	More  bool             `json:"more"`
}

var phoneSearch = regexp.MustCompile(`^[+0-9() -]+$`)
var phoneSearchSeparators = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "")

func normalizeAdminPhoneSearch(query string) string {
	if !phoneSearch.MatchString(query) {
		return ""
	}
	phone := phoneSearchSeparators.Replace(query)
	if strings.HasPrefix(phone, "0") && len(phone) > 1 {
		return "+62" + phone[1:]
	}
	return helper.NormalizePhoneID(phone)
}

func ListAdminNumbers(ctx context.Context, query, reputation string, offset int) (AdminNumberPage, error) {
	const pageSize = 50
	result := AdminNumberPage{Items: []AdminNumberRow{}}
	query = strings.TrimSpace(query)
	normalized := normalizeAdminPhoneSearch(query)
	if MongoConnected && MongoDB != nil {
		filter := bson.M{}
		if reputation != "" {
			filter["reputation_level"] = reputation
		}
		if query != "" {
			literal := primitive.Regex{Pattern: regexp.QuoteMeta(query), Options: "i"}
			alternatives := bson.A{bson.M{"phone_e164": literal}, bson.M{"top_community_label": literal},
				bson.M{"primary_category_label": literal}, bson.M{"primary_category": literal}, bson.M{"tags.tag": literal}}
			if normalized != "" && normalized != query {
				alternatives = append(alternatives, bson.M{"phone_e164": primitive.Regex{Pattern: regexp.QuoteMeta(normalized)}})
			}
			filter["$or"] = alternatives
		}
		opts := options.Find().SetSort(bson.D{{Key: "phone_e164", Value: 1}}).SetSkip(int64(offset)).SetLimit(pageSize + 1).
			SetProjection(bson.M{"_id": 0, "phone_e164": 1, "reputation_level": 1, "primary_category_label": 1,
				"top_community_label": 1, "tags": 1, "total_reports": 1})
		cursor, err := MongoDB.Collection(ColPhoneTags).Find(ctx, filter, opts)
		if err != nil {
			return result, err
		}
		if err := cursor.All(ctx, &result.Items); err != nil {
			return result, err
		}
	} else {
		memMu.RLock()
		defer memMu.RUnlock()
		term := strings.ToLower(query)
		for _, item := range memPhoneTags {
			if reputation != "" && string(item.ReputationLevel) != reputation {
				continue
			}
			fields := []string{item.PhoneE164, item.TopCommunityLabel, item.PrimaryCategoryLabel, string(item.PrimaryCategory)}
			for _, tag := range item.Tags {
				fields = append(fields, tag.Tag)
			}
			match := normalized != "" && strings.Contains(item.PhoneE164, normalized)
			for _, field := range fields {
				match = match || strings.Contains(strings.ToLower(field), term)
			}
			if match {
				result.Items = append(result.Items, AdminNumberRow{item.PhoneE164, item.ReputationLevel, item.PrimaryCategoryLabel, item.TopCommunityLabel, item.Tags, item.TotalReports})
			}
		}
		sort.Slice(result.Items, func(i, j int) bool { return result.Items[i].Phone < result.Items[j].Phone })
		result.Items = result.Items[min(offset, len(result.Items)):]
	}
	result.More = len(result.Items) > pageSize
	if result.More {
		result.Items = result.Items[:pageSize]
	}
	return result, nil
}
