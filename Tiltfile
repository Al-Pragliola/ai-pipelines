# AI Pipelines — Tilt dev environment
#
# Prerequisites:
#   GITHUB_TOKEN=$(gh auth token) make kind-setup
#
# Then:
#   tilt up

# Ignore generated files and local data to prevent feedback loops
watch_settings(ignore=['config/crd/bases/', 'bin/', 'cmd/dashboard/dist/', '.data/'])

# --- CRDs ---
# Re-install CRDs when API types change
local_resource(
    'crd-install',
    cmd='make install',
    deps=['api/v1alpha1/'],
)

# --- Pipeline CRs (local/ is gitignored — copy from config/samples/ and fill in your values) ---
local_resource(
    'sample-crs',
    cmd='kubectl apply -f local/',
    deps=['local/'],
    resource_deps=['crd-install'],
)

# --- Shared local data directory for SQLite history DB ---
local_resource(
    'data-dir',
    cmd='mkdir -p .data',
)

# --- Controller ---
local_resource(
    'controller',
    serve_cmd='go run ./cmd/main.go --history-db-path=.data/history.db --log-file=.data/operator.log',
    deps=[
        'cmd/main.go',
        'internal/controller/',
        'internal/issuehistory/',
        'api/v1alpha1/',
    ],
    resource_deps=['crd-install', 'sample-crs', 'data-dir'],
)

# --- Dashboard API ---
local_resource(
    'dashboard-api',
    serve_cmd='go run ./cmd/dashboard/main.go --history-db-path=.data/history.db --log-file=.data/operator.log',
    deps=[
        'cmd/dashboard/main.go',
        'internal/dashboard/',
        'internal/issuehistory/',
        'api/v1alpha1/',
    ],
    resource_deps=['crd-install', 'data-dir'],
    links=['http://localhost:9090'],
)

# --- Dashboard UI (Vite dev server) ---
local_resource(
    'dashboard-ui',
    serve_cmd='npm run dev',
    serve_dir='dashboard',
    deps=[
        'dashboard/src/',
        'dashboard/index.html',
    ],
    links=['http://localhost:5173'],
)
