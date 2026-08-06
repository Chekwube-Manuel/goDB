# DB Host API Documentation

## Authentication
All API requests (except `/health` and `/node/*`) require HTTP Basic Auth.
`POST /nodes` (node registration) additionally accepts the shared node token
via `Authorization: Bearer <NODE_TOKEN>`. The laptop accepts either admin
Basic Auth or the node bearer token, since the cloud forwards requests with
the node token.

## Endpoints

### Health
GET /health - Check service status (no auth required)

### Tenants
POST /tenants - Create a new tenant
GET /tenants - List all tenants

### Collections
POST /tenants/{slug}/collections - Create collection
GET /tenants/{slug}/collections - List collections

### Records
POST /tenants/{slug}/collections/{name}/records - Store record
GET /tenants/{slug}/collections/{name}/records - List records
(Requires the tenant and collection to exist.)

### Nodes
POST /nodes - Register a data node (agent, Bearer token)
GET /nodes - List registered nodes
POST /node/heartbeat - Node heartbeat (Bearer token, `/node/*` routes)

### Databases
POST /databases/provision - Provision a database (cloud: forwards to the target node, which creates a real PostgreSQL database; laptop: creates it locally)
GET /databases/status - List provisioned databases

### Dashboard
GET / or /dashboard - Minimal admin dashboard (Basic Auth)
