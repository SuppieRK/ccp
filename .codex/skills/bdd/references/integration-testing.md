# Integration Tests

Integration tests with Ginkgo follow similar patterns to unit tests but interact with real external services.

## Contents

- [Directory Structure](#directory-structure)
- [Integration Test Example](#integration-test-example)
- [Shared Integration Helpers](#shared-integration-helpers)

## Directory Structure

```
test-integration/
├── user/
│   ├── user_suite_test.go
│   └── postgres_test.go
├── helpers/
│   ├── postgres.go
│   └── kafka.go
└── README.md
```

## Integration Test Example

```go
// test-integration/user/user_suite_test.go
package user_test

import (
    "testing"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"
)

func TestUser(t *testing.T) {
    RegisterFailHandler(Fail)
    RunSpecs(t, "User Integration Suite")
}
```

```go
// test-integration/user/postgres_test.go
package user_test

import (
    "context"

    . "github.com/onsi/ginkgo/v2"
    . "github.com/onsi/gomega"

    "myservice/internal/user"
    "myservice/test-integration/helpers"
)

var _ = Describe("UserRepository with Postgres", func() {
    var (
        db   *sql.DB
        repo user.Repository
        ctx  context.Context
    )

    BeforeSuite(func() {
        if testing.Short() {
            Skip("skipping integration test in short mode")
        }

        var err error
        db, err = helpers.SetupTestDB()
        Expect(err).ToNot(HaveOccurred())

        DeferCleanup(func() {
            db.Close()
        })
    })

    BeforeEach(func() {
        repo = user.NewPostgresRepository(db)
        ctx = context.Background()

        // Clean up test data
        _, err := db.Exec("TRUNCATE TABLE users CASCADE")
        Expect(err).ToNot(HaveOccurred())
    })

    Describe("Save and FindByID", func() {
        It("persists and retrieves a user", func() {
            u := &user.User{
                ID:    "test-user-1",
                Name:  "Alice",
                Email: "alice@example.com",
            }

            err := repo.Save(ctx, u)
            Expect(err).ToNot(HaveOccurred())

            retrieved, err := repo.FindByID(ctx, "test-user-1")
            Expect(err).ToNot(HaveOccurred())
            Expect(retrieved.Email).To(Equal("alice@example.com"))
            Expect(retrieved.Name).To(Equal("Alice"))
        })

        It("returns error for non-existent user", func() {
            _, err := repo.FindByID(ctx, "non-existent")
            Expect(err).To(MatchError(user.ErrNotFound))
        })
    })
})
```

## Shared Integration Helpers

```go
// test-integration/helpers/postgres.go
package helpers

import (
    "database/sql"
    "os"

    _ "github.com/jackc/pgx/v5/stdlib"
)

func SetupTestDB() (*sql.DB, error) {
    dsn := os.Getenv("TEST_DATABASE_URL")
    if dsn == "" {
        return nil, fmt.Errorf("TEST_DATABASE_URL not set")
    }

    db, err := sql.Open("pgx", dsn)
    if err != nil {
        return nil, fmt.Errorf("failed to open database: %w", err)
    }

    if err := db.Ping(); err != nil {
        return nil, fmt.Errorf("failed to ping database: %w", err)
    }

    return db, nil
}
```
