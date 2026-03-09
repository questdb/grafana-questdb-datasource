# QuestDB Grafana Plugin - Release Guide

This document covers the full process for testing, packaging, and releasing a new version of the QuestDB Grafana datasource plugin.

## Prerequisites

- Node.js (18 is the version expected by the Grafana builder)
- Yarn
- Go (1.21+)
- Mage (Go build tool) - install via `go install github.com/magefile/mage@latest`
- Docker
- `microsocks` (for PDC testing) - install via `brew install microsocks`
- `gh` CLI (GitHub CLI)
- An account on Grafana Cloud, added to the QuestDB org (ask Sandro to add you)
- A personal `GRAFANA_ACCESS_POLICY_TOKEN` for local signing (see section 6)

## About the Plugin

The repo is at https://github.com/questdb/grafana-questdb-datasource. This is a "backend plugin", meaning it has both a frontend (TypeScript) and a backend (Go). The published plugin lives at https://grafana.com/orgs/questdb/plugins.

## 1. Prepare the Release

### 1.1 Ensure main is up to date

```bash
git checkout main
git pull
```

### 1.2 Bump the version

Edit `package.json` and update the `"version"` field (e.g. `"0.1.7"`).

> **Important:** The tag version must match the `package.json` version property. The CI release action validates this.

> **Note:** `src/plugin.json` uses `%VERSION%` which is replaced at build time by CI. Do not hardcode a version there.

### 1.3 Install frontend dependencies

```bash
yarn install
```

## 2. Build

### 2.1 Update dependencies and fix vulnerabilities

Before releasing, make sure dependencies are up to date. The CI plugin validator will **fail the release** if:
- The Grafana Go SDK is older than 5 months
- `osv-scanner` finds high severity vulnerabilities in `go.mod` or `yarn.lock`

**Go dependencies:**
```bash
go get -u github.com/grafana/grafana-plugin-sdk-go
go mod tidy
go test -v -count=1 ./...
```

**npm vulnerabilities:**
```bash
yarn audit
```

If there are high severity issues, add resolutions in `package.json` to force patched versions. For example:

```json
"resolutions": {
  "minimatch": "^3.1.3",
  "serialize-javascript": "^7.0.4"
}
```

Then `yarn install` and verify the build and tests still pass.

### 2.2 Frontend build

```bash
yarn build
```

Should compile with no errors. Bundle size should be around 400 KiB. Warnings about performance recommendations are expected.

### 2.2 Backend build

Use `mage` to build the backend for all platforms:

```bash
$(go env GOPATH)/bin/mage buildAll
```

This produces binaries in the `dist/` directory for all supported platforms (linux/amd64, linux/arm64, darwin/amd64, darwin/arm64, windows/amd64).

If you only need specific platforms for local Docker testing, you can build individually:

```bash
# For Apple Silicon Macs running arm64 Docker containers:
GOOS=linux GOARCH=arm64 go build -o dist/gpx_questdb_linux_arm64 ./pkg

# For amd64 Docker images (e.g. Grafana React 19 preview):
GOOS=linux GOARCH=amd64 go build -o dist/gpx_questdb_linux_amd64 ./pkg
```

At this point, the `dist/` directory contains the built plugin and you can use it to test a Grafana instance pointing to that directory as the plugins folder.

## 3. Run Tests

### 3.1 Frontend unit tests

```bash
npx jest --maxWorkers=2
```

All test suites should pass. As of v0.1.7 there are 22 suites with 290 tests.

> **Tip:** If tests OOM on a memory-constrained machine, reduce workers to 1 or run suites in batches.

### 3.2 Backend tests

```bash
go test -v -count=1 ./...
```

The `-count=1` flag forces the full suite to run, ignoring cache. For incremental changes during development, plain `go test ./...` is fine.

## 4. Integration Testing with Docker

Grafana will complain about the plugin being unsigned, which you can skip during development by passing `-e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=questdb-questdb-datasource`. With the `docker-compose.yml`, the env var `GF_DEFAULT_APP_MODE=development` is set, which also allows unsigned plugins.

### 4.1 Using docker-compose (with QuestDB in Docker)

The repo includes `docker-compose.yml` which starts Grafana and QuestDB together. The entire repo is mounted as the plugin directory, and a datasource is auto-provisioned.

```bash
GR_VERSION=12.3.0 docker compose up -d
```

Check `src/plugin.json` for the `grafanaDependency` value to know the minimum supported version (e.g. `>=12.3.0`).

### 4.2 Using docker run (standalone)

If you have QuestDB running locally and want more control, use `docker run` directly:

```bash
docker run --rm -it \
  -p 3000:3000 \
  -v $(pwd)/dist:/var/lib/grafana/plugins/questdb-questdb-datasource \
  -e GF_INSTALL_PLUGINS="" \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=questdb-questdb-datasource \
  grafana/grafana
```

If you need to pass any special config, you can create a file (do NOT commit it!) such as `test_grafana.ini` and mount a volume:

```bash
docker run --rm -it \
  -p 3000:3000 \
  -v $(pwd)/dist:/var/lib/grafana/plugins/questdb-questdb-datasource \
  -e GF_INSTALL_PLUGINS="" \
  -e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=questdb-questdb-datasource \
  -v $(pwd)/test_grafana.ini:/etc/grafana/grafana.ini \
  grafana/grafana
```

With `docker run`, you configure the datasource manually via the Grafana UI at http://localhost:3000 (default login: admin/admin).

### 4.3 Verify plugin loaded

Wait about 10 seconds for startup, then:

```bash
# Check Grafana is running
curl -s http://localhost:3000/api/health

# Check datasource is provisioned (only with docker-compose)
curl -s -u admin:admin http://localhost:3000/api/datasources

# Check plugin loaded in logs (should see "Plugin registered" with no errors)
docker compose logs grafana | grep -i -E "plugin.*questdb|plugin.*register|plugin.*load"
```

### 4.4 Create test data

The Docker QuestDB is available on port 9000 (REST API) and 8812 (PostgreSQL wire protocol):

```bash
# Create a test table
curl -s "http://localhost:9000/exec?query=CREATE+TABLE+IF+NOT+EXISTS+test_data+(ts+TIMESTAMP,+val+DOUBLE,+sym+SYMBOL)+timestamp(ts)+PARTITION+BY+DAY"

# Insert test rows
curl -s "http://localhost:9000/exec?query=INSERT+INTO+test_data+VALUES('2026-03-09T10:00:00.000000Z',+1.5,+'AAA'),('2026-03-09T10:01:00.000000Z',+2.3,+'BBB'),('2026-03-09T10:02:00.000000Z',+3.1,+'AAA'),('2026-03-09T10:03:00.000000Z',+4.7,+'BBB'),('2026-03-09T10:04:00.000000Z',+5.2,+'AAA')"
```

### 4.5 Run test queries

Test key QuestDB query patterns through the Grafana datasource proxy. The datasource ID is typically 1 (check via the datasources API above).

**Basic SELECT:**
```bash
curl -s -u admin:admin -X POST http://localhost:3000/api/ds/query \
  -H "Content-Type: application/json" \
  -d '{"queries":[{"refId":"A","datasourceId":1,"rawSql":"SELECT * FROM test_data","format":1,"queryType":"sql","selectedFormat":4}],"from":"now-1h","to":"now"}'
```
Expected: 5 rows with ts, val, sym fields.

**SAMPLE BY (time bucketing):**
```bash
curl -s -u admin:admin -X POST http://localhost:3000/api/ds/query \
  -H "Content-Type: application/json" \
  -d '{"queries":[{"refId":"A","datasourceId":1,"rawSql":"SELECT ts, avg(val) FROM test_data SAMPLE BY 2m","format":1,"queryType":"sql","selectedFormat":4}],"from":"now-1h","to":"now"}'
```
Expected: 3 bucketed rows.

**LATEST ON (latest per partition):**
```bash
curl -s -u admin:admin -X POST http://localhost:3000/api/ds/query \
  -H "Content-Type: application/json" \
  -d '{"queries":[{"refId":"A","datasourceId":1,"rawSql":"SELECT ts, sym, val FROM test_data LATEST ON ts PARTITION BY sym","format":1,"queryType":"sql","selectedFormat":4}],"from":"now-1h","to":"now"}'
```
Expected: 2 rows (one per symbol).

**GROUP BY + ORDER BY:**
```bash
curl -s -u admin:admin -X POST http://localhost:3000/api/ds/query \
  -H "Content-Type: application/json" \
  -d '{"queries":[{"refId":"A","datasourceId":1,"rawSql":"SELECT sym, count(*), sum(val) FROM test_data GROUP BY sym ORDER BY sym","format":1,"queryType":"sql","selectedFormat":4}],"from":"now-1h","to":"now"}'
```
Expected: AAA (count=3, sum=9.8), BBB (count=2, sum=7).

### 4.6 Test with latest/next Grafana

```bash
docker compose down
```

For Grafana 13 / React 19 preview (if available), the preview image is under `grafana/grafana` (not `grafana-enterprise`). You may need to temporarily edit `docker-compose.yml` to change the image name, e.g. `grafana/grafana:dev-preview-react19`.

If the preview image is amd64-only, make sure you have the amd64 backend binary:

```bash
GOOS=linux GOARCH=amd64 go build -o dist/gpx_questdb_linux_amd64 ./pkg
```

Repeat all the same test queries from section 4.5.

### 4.7 Clean up Docker images

After testing, remove the downloaded Grafana images to save disk space:

```bash
docker compose down
docker rmi grafana/grafana-enterprise:12.3.0
# Remove any other test images you pulled
```

## 5. PDC (Private Data Connect) Testing

PDC allows Grafana Cloud to reach private databases via a SOCKS5 proxy. This tests that the plugin correctly routes connections through a proxy.

### 5.1 Start microsocks

```bash
microsocks -p 1080 &
```

### 5.2 Start QuestDB locally

Make sure QuestDB is running on your host machine (port 8812 for PostgreSQL wire protocol, port 9000 for REST API).

### 5.3 Create PDC provisioning file

Create `provisioning/datasources/questdb_pdc_test.yaml` (do NOT commit this):

```yaml
apiVersion: 1
datasources:
  - name: QuestDB-PDC-test
    type: questdb-questdb-datasource
    jsonData:
      server: host.docker.internal
      port: 8812
      username: admin
      tlsMode: disable
      maxOpenConnections: 100
      maxIdleConnections: 100
      maxConnectionLifetime: 14400
      enableSecureSocksProxy: true
    secureJsonData:
      password: quest
```

### 5.4 Create PDC Docker Compose file

Create `docker-compose.pdc.yml` (do NOT commit this):

```yaml
version: '3.7'
services:
  grafana:
    image: grafana/grafana-enterprise:${GR_VERSION:-12.3.0}
    ports:
      - '3000:3000'
    volumes:
      - ./:/var/lib/grafana/plugins/questdb-questdb-datasource
      - ./provisioning:/etc/grafana/provisioning
    environment:
      - TERM=linux
      - GF_DEFAULT_APP_MODE=development
      - GF_SECURE_SOCKS_DATASOURCE_PROXY_SERVER_ENABLED=true
      - GF_SECURE_SOCKS_DATASOURCE_PROXY_PROXY_ADDRESS=host.docker.internal:1080
      - GF_SECURE_SOCKS_DATASOURCE_PROXY_ALLOW_INSECURE=true
    extra_hosts:
      - "host.docker.internal:host-gateway"
    networks:
      - grafana

networks:
  grafana:
```

Key environment variables:
- `GF_SECURE_SOCKS_DATASOURCE_PROXY_SERVER_ENABLED=true` - enables the SOCKS proxy feature in Grafana
- `GF_SECURE_SOCKS_DATASOURCE_PROXY_PROXY_ADDRESS=host.docker.internal:1080` - where microsocks is listening
- `GF_SECURE_SOCKS_DATASOURCE_PROXY_ALLOW_INSECURE=true` - allows plain SOCKS5 (no TLS), needed for microsocks

### 5.5 Start and test

```bash
docker compose -f docker-compose.pdc.yml up -d
```

Wait about 10 seconds, then:

```bash
# Verify health
curl -s http://localhost:3000/api/health

# Check both datasources are provisioned (PDC one should show enableSecureSocksProxy: true)
curl -s -u admin:admin http://localhost:3000/api/datasources

# Run a query through the PDC datasource (use the datasource ID for QuestDB-PDC-test)
curl -s -u admin:admin -X POST http://localhost:3000/api/ds/query \
  -H "Content-Type: application/json" \
  -d '{"queries":[{"refId":"A","datasourceId":1,"rawSql":"SELECT * FROM some_table LIMIT 3","format":1,"queryType":"sql","selectedFormat":4}],"from":"now-30d","to":"now"}'
```

If the query returns data from your local QuestDB, PDC is working. The traffic flows: Grafana container -> SOCKS5 proxy (microsocks on host:1080) -> local QuestDB (host:8812).

### 5.6 Clean up PDC test

```bash
docker compose -f docker-compose.pdc.yml down
pkill microsocks
rm docker-compose.pdc.yml provisioning/datasources/questdb_pdc_test.yaml
docker rmi grafana/grafana-enterprise:12.3.0
```

## 6. Sign the Plugin

Signing is optional during development, but mandatory to submit to Grafana.

### 6.1 Get an access policy token

1. Go to https://grafana.com/orgs/questdb/access-policies
2. You should see an existing `plugin-signing` item, which has the scopes you need
3. Click "Add token" to create a personal token - keep it safe!
4. There is an already generated token that is used automatically by GitHub Actions to sign the plugin when creating a release (stored as `GRAFANA_ACCESS_POLICY_TOKEN` repo secret). But you will need your own token to test everything yourself.

### 6.2 Set the token and sign

```bash
export GRAFANA_ACCESS_POLICY_TOKEN=YOUR_TOKEN
npx --yes @grafana/sign-plugin@latest
```

This signs the contents of the `dist/` directory from the root of the repo, creating a `MANIFEST.txt` file inside it.

### 6.3 Testing locally with a signed plugin

Once signed, you can test with Grafana without the `ALLOW_LOADING_UNSIGNED_PLUGINS` flag:

```bash
docker run --rm -it \
  -p 3000:3000 \
  -v $(pwd)/dist:/var/lib/grafana/plugins/questdb-questdb-datasource \
  -e GF_INSTALL_PLUGINS="" \
  grafana/grafana
```

If the plugin loads without the unsigned warning, signing was successful. Grafana logs should show the plugin registered without any signature warnings.

## 7. Package for Manual Submission

Once you are happy, you can generate a zip file manually to submit to Grafana. You rename the dist folder, zip it, and validate it:

### 7.1 Build everything (if not already done)

```bash
yarn install
yarn build
$(go env GOPATH)/bin/mage buildAll
```

### 7.2 Sign the plugin (if not already done)

```bash
export GRAFANA_ACCESS_POLICY_TOKEN=YOUR_TOKEN
npx --yes @grafana/sign-plugin@latest
```

### 7.3 Create and validate the zip

```bash
mv dist questdb-questdb-datasource
zip -r questdb-questdb-datasource.zip questdb-questdb-datasource
npx @grafana/plugin-validator@latest -sourceCodeUri file://./ questdb-questdb-datasource.zip
```

If no errors, your plugin is ready. Warnings are fine - use your common sense, but the plugin does have some warnings and is approved.

### 7.4 Restore dist directory

```bash
mv questdb-questdb-datasource dist
```

> **Important:** Do not commit the zip file or the renamed directory.

## 8. Release via CI (Recommended)

The preferred release method is via GitHub Actions, which handles building, signing, and packaging automatically. **You do NOT need to sign locally or generate a personal token when using this flow.** CI uses the `GRAFANA_ACCESS_POLICY_TOKEN` repo secret to sign the plugin. All you need to do is bump the version, commit, tag, and push.

### 8.1 CI Actions Overview

There are two GitHub Actions on the plugin repo:
1. **Test workflow** (`.github/workflows/run-tests.yml`) - runs on pushes, executes the test suite
2. **Release workflow** (`.github/workflows/release.yml`) - runs only when you create a new tag, builds/signs/packages the plugin

Even though these actions replicate what you can do manually, it is a good idea to be able to run them beforehand on a fork (see section 8.2).

### 8.2 Testing CI on a Fork (Recommended)

Fork the repo so you can create tags on your fork and verify everything works before touching the main repo.

```bash
# Add your fork as a remote
git remote add myfork https://github.com/YOUR_USERNAME/grafana-questdb-datasource.git
```

Then on your forked repo on GitHub:
1. Go to Settings > Secrets and add a repository secret named `GRAFANA_ACCESS_POLICY_TOKEN` with your Grafana signing token
2. Go to Actions and enable both workflows (by default, actions are not enabled on forked repos)

Now you can push changes to test the regular workflow:
```bash
git push -u myfork your_branch
```

And create tags to test the signing/packaging action:
```bash
git tag v0.1.7
git push myfork v0.1.7
```

> **Important:** The tag version must match the `package.json` version property.

If you need to reuse a tag during development on the fork:
```bash
git tag -f v0.1.7
git push myfork v0.1.7 --force
```

### 8.3 Release on the Main Repo

Once you are happy that everything works on the fork and both test and package actions succeed, merge to main.

```bash
git add package.json
git commit -m "Bump version to 0.1.7"
git push origin main
git tag v0.1.7
git push origin v0.1.7
```

### 8.4 What CI Does

On tag create, the "release" action triggers. After a while (~7 minutes), you should see the action succeeded. At this point, the action will have:
1. Built the frontend (`yarn install && yarn build`)
2. Built the backend for all platforms via mage
3. Signed the plugin using the `GRAFANA_ACCESS_POLICY_TOKEN` repo secret
4. Created a **draft release** with both a `.zip` file and a `.sha` file

### 8.5 Publish the Release

The draft release has a template explaining what to do. You can remove everything from the template and just leave the changelog.

The draft release explains that the zip and sha file URLs might not be active until you publish the release. Since you need those files to submit the plugin to Grafana, **you need to publish the release at this point**. Once published, both links will work.

## 9. Submit to Grafana Plugin Registry

1. Go to https://grafana.com/orgs/questdb/plugins and select "update plugin"
2. The form expects:
   - The **zip URL** from the GitHub release
   - The **sha URL** from the GitHub release
   - The **root URL** of the GitHub repo
   - **Testing guidance** - describe what changed and how to test. Example:

     > This release adds Grafana 13/React 19 compatibility and switches to the new @questdb/sql-parser. To test, start a QuestDB instance via `docker run -p 8812:8812 questdb/questdb`, then on Grafana create a datasource pointing to localhost or host.docker.internal, port 8812, user admin, password quest and TLS options disable. More info at https://questdb.com/docs/third-party-tools/grafana/

3. This triggers Grafana's review process. You should get automated emails, and shortly after the plugin should be approved.

> **Note:** The `grafanaDependency` field in `src/plugin.json` controls which Grafana versions can install this plugin version. Users on older Grafana versions will automatically get the last compatible plugin version from the registry.

## Key Files Reference

| File | Purpose |
|------|---------|
| `package.json` | Version number (must match tag), frontend dependencies |
| `src/plugin.json` | Plugin metadata, `grafanaDependency`, `%VERSION%` placeholder |
| `Magefile.go` | Backend build targets (used by `mage buildAll`) |
| `.github/workflows/release.yml` | CI release workflow (triggers on `v*` tags) |
| `.github/workflows/run-tests.yml` | CI test workflow (triggers on push) |
| `docker-compose.yml` | Local integration testing (Grafana + QuestDB) |
| `provisioning/datasources/` | Datasource provisioning for Docker testing |
| `.config/webpack/webpack.config.ts` | Webpack config (externals for React 19) |
| `pkg/plugin/driver.go` | Backend - database connection, PDC/SOCKS proxy support |

## Troubleshooting

- **Plugin not loading in Docker:** Check `docker compose logs grafana | grep -i plugin`. With docker-compose, `GF_DEFAULT_APP_MODE=development` allows unsigned plugins. With `docker run`, use `-e GF_PLUGINS_ALLOW_LOADING_UNSIGNED_PLUGINS=questdb-questdb-datasource`.
- **Backend binary not found:** Make sure you built with `mage buildAll` or the correct `GOOS`/`GOARCH`. Docker on Apple Silicon uses `linux/arm64` by default, but some preview images are `linux/amd64` only.
- **PDC query fails with "connection refused":** Verify microsocks is running (`pgrep microsocks`), QuestDB is accessible on port 8812, and `host.docker.internal` resolves inside the container (`extra_hosts` in compose).
- **Signing fails:** Check your `GRAFANA_ACCESS_POLICY_TOKEN` is set and has the correct scopes. Generate one at https://grafana.com/orgs/questdb/access-policies under the existing `plugin-signing` policy.
- **Validator fails:** Common issues are missing `MANIFEST.txt` (plugin not signed) or mismatched plugin ID. The zip must contain a single directory named `questdb-questdb-datasource`.
- **Tag version mismatch:** The CI release action validates that the tag matches `package.json` version. Update the file before tagging.
- **CI actions not running on fork:** By default, actions are disabled on forks. Go to the Actions tab and enable them manually.
