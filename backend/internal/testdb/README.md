# testdb

Test helper. `testdb.New(t)` creates a fresh, uniquely named database on the server in `TEST_DATABASE_URL`, returns a pool for it and drops it when the test ends (`t.Cleanup`). Tests are isolated and order-independent. Without `TEST_DATABASE_URL` the test is skipped, so `go test ./...` works without Postgres.

```bash
TEST_DATABASE_URL='postgres://app:app@localhost:5432/app?sslmode=disable' go test -race ./...
```
