package config

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/CallTag/backend/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GoogleClientID adalah client ID jenis Web application untuk audience ID token.
func GoogleClientID() string { return strings.TrimSpace(os.Getenv("GOOGLE_CLIENT_ID")) }

// FindOrCreateGoogleUser mengikat akun ke sub Google yang sudah diverifikasi.
func FindOrCreateGoogleUser(ctx context.Context, subject, name, email string) (model.User, error) {
	if !MongoConnected || MongoDB == nil {
		return model.User{}, errors.New("login Google membutuhkan MongoDB")
	}
	now := time.Now().UTC()
	col := MongoDB.Collection(ColUsers)
	filter := bson.M{"google_sub": subject, "status": "ACTIVE"}
	update := bson.M{
		"$set":         bson.M{"display_name": name, "email": email, "updated_at": now},
		"$setOnInsert": bson.M{"google_sub": subject, "status": "ACTIVE", "role": "USER", "trust_weight": 1.0, "token_version": 1, "created_at": now},
	}
	var user model.User
	opts := options.FindOneAndUpdate().SetReturnDocument(options.After)
	err := col.FindOneAndUpdate(ctx, filter, update, opts.SetUpsert(true)).Decode(&user)
	if mongo.IsDuplicateKeyError(err) {
		err = col.FindOneAndUpdate(ctx, filter, update, opts.SetUpsert(false)).Decode(&user)
	}
	return user, err
}
