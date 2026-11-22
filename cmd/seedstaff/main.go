// Command seedstaff creates a durable staff account without invoking demo seed initialization.
package main

import (
	"bufio"
	"context"
	"crypto/rand"
	"encoding/base32"
	"errors"
	"flag"
	"fmt"
	"github.com/CallTag/backend/config"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"io"
	"net/url"
	"os"
	"strings"
	"time"
)

func main() {
	if err := run(os.Args[1:], os.Stdin, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "seedstaff:", err)
		os.Exit(1)
	}
}
func run(args []string, in io.Reader, out io.Writer) error {
	flags := flag.NewFlagSet("seedstaff", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	enroll := flags.Bool("enroll", false, "intentionally display a new TOTP enrollment URI")
	rename := flags.Bool("rename-from-email", false, "assign a username to an existing staff account")
	if flags.Parse(args) != nil || flags.NArg() != 0 || (*enroll && *rename) {
		return errors.New("usage: seedstaff [-enroll | -rename-from-email]; supply credentials via STAFF_SEED_* environment or piped stdin")
	}
	if config.GetMongoURI() == "" {
		return errors.New("MONGOSTRING required; staff seed never writes ephemeral memory")
	}
	reader := bufio.NewReader(in)
	read := func(name string) (string, error) {
		if value := os.Getenv(name); value != "" {
			return value, nil
		}
		if file, ok := in.(*os.File); ok {
			info, err := file.Stat()
			if err != nil {
				return "", err
			}
			if info.Mode()&os.ModeCharDevice != 0 {
				return "", fmt.Errorf("set %s via secret environment or provide piped stdin (interactive echo disabled)", name)
			}
		}
		fmt.Fprintln(out, "Read", name, "from stdin:")
		value, err := reader.ReadString('\n')
		if err != nil && err != io.EOF {
			return "", err
		}
		if strings.TrimSpace(value) == "" {
			return "", fmt.Errorf("%s required", name)
		}
		return strings.TrimSuffix(strings.TrimSuffix(value, "\n"), "\r"), nil
	}
	var legacyEmail string
	if *rename {
		var err error
		legacyEmail, err = read("STAFF_LEGACY_EMAIL")
		if err != nil {
			return err
		}
	}
	username, err := read("STAFF_SEED_USERNAME")
	if err != nil {
		return err
	}
	username = config.NormalizeStaffUsername(username)
	var password, role, secret string
	if !*rename {
		password, err = read("STAFF_SEED_PASSWORD")
		if err != nil {
			return err
		}
		role, err = read("STAFF_SEED_ROLE")
		if err != nil {
			return err
		}
		if *enroll {
			key := make([]byte, 20)
			if _, err := rand.Read(key); err != nil {
				return err
			}
			secret = base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(key)
		} else {
			secret, err = read("STAFF_SEED_TOTP_SECRET")
			if err != nil {
				return err
			}
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx, options.Client().ApplyURI(config.GetMongoURI()))
	if err != nil {
		return errors.New("MongoDB connection failed")
	}
	defer client.Disconnect(context.Background())
	if err := client.Ping(ctx, nil); err != nil {
		return errors.New("MongoDB unavailable")
	}
	config.MongoClient = client
	config.MongoDB = client.Database(config.GetDBName())
	config.MongoConnected = true
	defer func() { config.MongoClient = nil; config.MongoDB = nil; config.MongoConnected = false }()
	if err := config.EnsureStaffIndexes(ctx); err != nil {
		return errors.New("staff indexes failed")
	}
	if *rename {
		if err := config.SetLegacyStaffUsername(ctx, legacyEmail, username); err != nil {
			return errors.New("username assignment rejected: check legacy account and choose a valid, available username")
		}
		fmt.Fprintln(out, "Staff username assigned:", username)
		return nil
	}
	if err := config.CreateStaff(ctx, username, password, role, secret); err != nil {
		return errors.New("staff creation rejected: check username, unique account, password (8–72 bytes), role, and TOTP secret")
	}
	if *enroll {
		u := url.URL{Scheme: "otpauth", Host: "totp", Path: "/CallTag:" + username}
		query := url.Values{"secret": {secret}, "issuer": {"CallTag"}, "algorithm": {"SHA1"}, "digits": {"6"}, "period": {"30"}}
		u.RawQuery = query.Encode()
		fmt.Fprintln(out, "TOTP enrollment (sensitive; store only in your authenticator):", u.String())
	}
	fmt.Fprintln(out, "Staff account created:", username)
	return nil
}
