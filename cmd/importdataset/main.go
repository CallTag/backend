// Command importdataset validates and imports curated CSV/JSON public datasets.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/CallTag/backend/config"
	"github.com/CallTag/backend/internal/dataset"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "importdataset:", err)
		os.Exit(1)
	}
}

func run(args []string, out io.Writer) error {
	flags := flag.NewFlagSet("importdataset", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	file := flags.String("file", "", "CSV/JSON file")
	kind := flags.String("kind", "", "verified|scam|tag_context")
	source := flags.String("source", "", "dataset provenance label; overrides per-row source")
	dryRun := flags.Bool("dry-run", false, "validate without connecting to MongoDB")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *file == "" || flags.NArg() != 0 {
		return errors.New("usage: importdataset -file <path> -kind verified|scam|tag_context [-source LABEL] [-dry-run]")
	}
	f, err := os.Open(*file)
	if err != nil {
		return err
	}
	defer f.Close()
	rows, err := dataset.Parse(f, filepath.Ext(*file), *kind, *source)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if !*dryRun {
		if config.GetMongoURI() == "" {
			return errors.New("MONGOSTRING is required for persistent import; use -dry-run for validation")
		}
		client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.GetMongoURI()).SetServerSelectionTimeout(8*time.Second))
		if err != nil {
			return err
		}
		defer client.Disconnect(context.Background())
		if err := client.Ping(ctx, nil); err != nil {
			return fmt.Errorf("MongoDB unavailable: %w", err)
		}
		db := client.Database(config.GetDBName())
		// Create only importer indexes; never run application demo seed initialization.
		if _, err := db.Collection(config.ColSpamReports).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "reporter_user_id", Value: 1}, {Key: "idempotency_key", Value: 1}}, Options: options.Index().SetUnique(true).SetName("idx_reports_idempotency").SetPartialFilterExpression(bson.M{"idempotency_key": bson.M{"$gt": ""}})}); err != nil {
			return err
		}
		if _, err := db.Collection(config.ColVerifiedNumbers).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "phone_e164", Value: 1}}, Options: options.Index().SetUnique(true).SetName("idx_verified_phone")}); err != nil {
			return err
		}
		if _, err := db.Collection(config.ColPhoneTags).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: bson.D{{Key: "phone_e164", Value: 1}}, Options: options.Index().SetUnique(true).SetName("idx_phonetags_e164")}); err != nil {
			return err
		}
		config.MongoClient = client
		config.MongoDB = db
		config.MongoConnected = true
		defer func() { config.MongoClient = nil; config.MongoDB = nil; config.MongoConnected = false }()
	}
	result, err := config.ImportDataset(ctx, rows, *kind, *dryRun)
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(result)
}
