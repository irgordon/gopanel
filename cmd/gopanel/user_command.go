package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/irgordon/gopanel/internal/auth"
	"github.com/irgordon/gopanel/internal/diagnostic"
	"github.com/irgordon/gopanel/internal/store"
)

const defaultUserDatabasePath = "./gopanel.db"

type userCommand struct {
	databasePath string
}

type adminCredentials struct {
	email    string
	name     string
	password string
}

func runUserCommand(arguments []string, diagnostics *diagnostic.Recorder) error {
	command, err := parseUserCommand(arguments)
	if err != nil {
		return err
	}
	credentials, err := readAdminCredentials(os.Stdin, os.Stdout, term.IsTerminal, term.ReadPassword)
	if err != nil {
		return err
	}
	if err := createAdmin(context.Background(), command.databasePath, credentials, diagnostics); err != nil {
		return err
	}
	_, err = fmt.Fprintln(os.Stdout, "Admin user created.")
	return err
}

func parseUserCommand(arguments []string) (userCommand, error) {
	if len(arguments) == 0 || arguments[0] != "create-admin" {
		return userCommand{}, errors.New("unknown user command; use: gopanel user create-admin [--database-path path]")
	}
	parsed := userCommand{databasePath: defaultUserDatabasePath}
	flags := flag.NewFlagSet("gopanel user create-admin", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	flags.StringVar(&parsed.databasePath, "database-path", defaultUserDatabasePath, "SQLite database path")
	if err := flags.Parse(arguments[1:]); err != nil {
		return userCommand{}, fmt.Errorf("parse create-admin arguments: %w", err)
	}
	if flags.NArg() != 0 {
		return userCommand{}, errors.New("unexpected create-admin positional arguments")
	}
	return parsed, nil
}

func readAdminCredentials(input *os.File, output io.Writer, isTerminal func(int) bool, readPassword func(int) ([]byte, error)) (adminCredentials, error) {
	if !isTerminal(int(input.Fd())) {
		return adminCredentials{}, errors.New("create-admin requires an interactive terminal")
	}
	reader := bufio.NewReader(input)
	email, err := readLine(reader, output, "Email: ")
	if err != nil {
		return adminCredentials{}, err
	}
	name, err := readLine(reader, output, "Name: ")
	if err != nil {
		return adminCredentials{}, err
	}
	if _, err := fmt.Fprint(output, "Password: "); err != nil {
		return adminCredentials{}, fmt.Errorf("write password prompt: %w", err)
	}
	passwordBytes, err := readPassword(int(input.Fd()))
	if err != nil {
		return adminCredentials{}, fmt.Errorf("read password securely: %w", err)
	}
	if _, err := fmt.Fprintln(output); err != nil {
		return adminCredentials{}, fmt.Errorf("finish password prompt: %w", err)
	}
	credentials := adminCredentials{email: strings.TrimSpace(email), name: strings.TrimSpace(name), password: string(passwordBytes)}
	if credentials.email == "" || credentials.name == "" || credentials.password == "" {
		return adminCredentials{}, errors.New("email, name, and password are required")
	}
	return credentials, nil
}

func readLine(reader *bufio.Reader, output io.Writer, prompt string) (string, error) {
	if _, err := fmt.Fprint(output, prompt); err != nil {
		return "", fmt.Errorf("write prompt: %w", err)
	}
	value, err := reader.ReadString('\n')
	if err != nil {
		return "", fmt.Errorf("read prompted value: %w", err)
	}
	return value, nil
}

func createAdmin(ctx context.Context, databasePath string, credentials adminCredentials, diagnostics *diagnostic.Recorder) (resultError error) {
	database, err := store.Open(ctx, databasePath)
	if err != nil {
		return recordUserCommandFailure(diagnostics, "database_open_failed", "SQLite could not be opened.", err)
	}
	defer func() {
		resultError = errors.Join(resultError, database.Close())
	}()
	if err := database.Migrate(ctx); err != nil {
		return recordUserCommandFailure(diagnostics, "migration_failed", "SQLite migration failed.", err)
	}
	normalized, err := auth.NormalizeEmail(credentials.email)
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(credentials.password)
	if err != nil {
		return err
	}
	authStore := auth.NewStore(database.SQLDatabase())
	if _, err := authStore.CreateUser(ctx, normalized, credentials.name, hash, "admin"); err != nil {
		return fmt.Errorf("create admin user: %w", err)
	}
	return nil
}

func recordUserCommandFailure(diagnostics *diagnostic.Recorder, event, message string, cause error) error {
	record := diagnostics.Record(diagnostic.Input{
		Event:           event,
		Component:       "sqlite",
		PublicMessage:   message,
		TechnicalDetail: "user command storage operation failed",
	})
	return startupFailure{message: message, reference: record.ID, cause: cause}
}
