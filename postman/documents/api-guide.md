# DB Host API Documentation

## Authentication
All API requests (except /node/*) require HTTP Basic Auth.

## Endpoints

### Health
GET /health - Check service status

### Tenants
POST /tenants - Create a new tenant
GET /tenants - List all tenants

### Collections
POST /tenants/{slug}/collections - Create collection
GET /tenants/{slug}/collections - List collections

### Records
POST /tenants/{slug}/collections/{name}/records - Store record
GET /tenants/{slug}/collections/{name}/records - List records

### Nodes
POST /nodes - Register a data node
GET /nodes - List registered nodes
POST /node/heartbeat - Node heartbeat

### Databases
POST /databases/provision - Provision a database
GET /databases/status - List provisioned databases
