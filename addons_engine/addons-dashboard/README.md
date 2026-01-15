# Oakestra Addons Management WebApp

A modern Angular web application for managing Oakestra addons, hooks, and custom resources.

## Features

### 📦 Marketplace Management
- View all addons available in the marketplace
- Add new addons to the marketplace with service configurations
- Delete addons from the marketplace
- Install addons directly from marketplace cards
- View detailed information about each addon

### 💾 Installed Addons Management
- View all installed addons in your cluster
- Install addons from the marketplace by ID
- Filter addons by status (Installing, Running, Failed, Disabling)
- Uninstall addons
- View detailed information and status

### 🪝 Hooks Management
- Add lifecycle hooks (pre-create, post-create, pre-update, post-update, pre-delete, post-delete)
- View all configured hooks
- Delete hooks
- Fully integrated with Resource Abstractor API

### 🔧 Custom Resources Management
- Add custom resource type definitions with JSON schemas
- View all custom resource types
- Fully integrated with Resource Abstractor API

## Technology Stack

- **Angular 17** - Modern standalone components
- **TypeScript** - Type-safe development
- **RxJS** - Reactive programming
- **nginx** - Production web server (Docker)

## Prerequisites

- **Oakestra Marketplace API** running (default: `http://localhost:11102`)
- **Oakestra Addons Engine API** running (default: `http://localhost:11101`)
- **Resource Abstractor API** running (default: `http://localhost:10000`)

For development:
- Node.js 20+ and npm

For Docker deployment:
- Docker

## Development Setup

### Install Dependencies

```bash
npm install
```

### Run Development Server

```bash
npm start
```

The application will be available at `http://localhost:4200` and will automatically reload when you make changes.

### Build for Production

```bash
npm run build:prod
```

The built files will be in the `dist/oakestra-addons-ui` directory.

## Docker Deployment

### Environment Variables

The application supports runtime configuration via environment variables. Each API endpoint is configured using three components:

**Marketplace API:**
- `MARKETPLACE_PROTOCOL` - Protocol (default: `http`)
- `MARKETPLACE_URL` - Hostname or IP (default: `localhost`)
- `MARKETPLACE_PORT` - Port number (default: `11102`)

**Addons Engine API:**
- `ADDONS_ENGINE_PROTOCOL` - Protocol (default: `http`)
- `ADDONS_ENGINE_URL` - Hostname or IP (default: `localhost`)
- `ADDONS_ENGINE_PORT` - Port number (default: `11101`)

**Resource Abstractor API:**
- `RESOURCE_ABSTRACTOR_PROTOCOL` - Protocol (default: `http`)
- `RESOURCE_ABSTRACTOR_URL` - Hostname or IP (default: `localhost`)
- `RESOURCE_ABSTRACTOR_PORT` - Port number (default: `10000`)

**Dashboard:**
- `DASHBOARD_PORT` - Port the dashboard listens on (default: `11103`)

These environment variables are injected at container startup and assembled into full URLs.

### Build the Docker Image

```bash
docker build -t oakestra-addons-ui:latest .
```

### Run the Container

```bash
docker run -d \
  --name oakestra-addons-ui \
  -p 11103:11103 \
  oakestra-addons-ui:latest
```

The application will be available at `http://localhost:11103`.

To use a different port, change both the `-p` mapping and `DASHBOARD_PORT`:

```bash
docker run -d \
  --name oakestra-addons-ui \
  -p 8080:8080 \
  -e DASHBOARD_PORT=8080 \
  oakestra-addons-ui:latest
```

## Configuration

The webapp supports three levels of configuration (in order of precedence):

1. **User Preferences** (Highest Priority) - Stored in browser localStorage, set via the gear icon menu
2. **Environment Variables** (Runtime) - Set via Docker environment variables at container startup
3. **Compiled Defaults** (Fallback) - Built into the application

### Accessing Configuration

Click the **gear icon** (⚙️) in the top-right corner of the header to access the configuration dropdown menu where you can:

- View current API endpoints
- Modify API URLs (saved to localStorage)
- Reset to environment/default values

## Usage

### Adding an Addon to the Marketplace

1. Go to the **Marketplace** tab
2. Click **+ Add New Addon**
3. Fill in the form:
   - **Name**: The name of your addon
   - **Description**: What your addon does
   - **Version**: Semantic version (e.g., 1.0.0)
   - **Services**: JSON array of service definitions
4. Click **Submit**

Example services JSON:
```json
[
  {
    "service_name": "my-alpine",
    "image": "alpine",
    "command": "sh -c 'while true; do echo \"Hello, World!\"; sleep 10; done'"
  },
  {
    "service_name": "redis-cache",
    "image": "redis:7-alpine",
    "ports": ["6379:6379"]
  }
]
```

### Installing an Addon

**Method 1**: From the Marketplace tab
1. Find the addon you want to install
2. Click the **Install** button on the addon card

**Method 2**: From the Installed Addons tab
1. Click **+ Install from Marketplace**
2. Enter the Marketplace Addon ID
3. Click **Install**

### Managing Installed Addons

1. Go to the **Installed Addons** tab
2. Use the filter buttons to view addons by status
3. Click **View Details** to see full addon information
4. Click **Uninstall** to remove an addon

### Managing Hooks

1. Go to the **Hooks** tab
2. Click **+ Add Hook** to create a new hook
3. Fill in:
   - **Hook Name**: Descriptive name
   - **Entity**: The entity type (e.g., application, service)
   - **Webhook URL**: The URL to be called when hook triggers
   - **Events**: Select one or more events (pre_create, post_create, etc.)
4. View all hooks and delete as needed

### Managing Custom Resources

1. Go to the **Custom Resources** tab
2. Click **+ Add Custom Resource** to define a new resource type
3. Fill in:
   - **Resource Type**: Unique identifier (e.g., database, cache)
   - **JSON Schema**: Schema to validate resource instances
4. View all resource type definitions

## API Integration

### Marketplace API Endpoints Used

- `GET /api/v1/marketplace/addons` - List all marketplace addons
- `POST /api/v1/marketplace/addons` - Add a new addon
- `GET /api/v1/marketplace/addons/:id` - Get addon details
- `DELETE /api/v1/marketplace/addons/:id` - Delete an addon

### Addons Engine API Endpoints Used

- `GET /api/v1/addons` - List all installed addons
- `POST /api/v1/addons` - Install an addon
- `GET /api/v1/addons/:id` - Get addon details
- `DELETE /api/v1/addons/:id` - Uninstall an addon

### Resource Abstractor API Endpoints Used

**Hooks:**
- `GET /api/v1/hooks` - List all hooks
- `POST /api/v1/hooks` - Add a new hook
- `GET /api/v1/hooks/:id` - Get hook details
- `PATCH /api/v1/hooks/:id` - Update a hook
- `DELETE /api/v1/hooks/:id` - Delete a hook

**Custom Resources:**
- `GET /api/v1/custom-resources` - List all custom resource types
- `POST /api/v1/custom-resources` - Add a new custom resource type
- `GET /api/v1/custom-resources/:type` - List instances of a resource type
- `POST /api/v1/custom-resources/:type` - Create a resource instance
- `GET /api/v1/custom-resources/:type/:id` - Get resource instance details
- `PATCH /api/v1/custom-resources/:type/:id` - Update a resource instance
- `DELETE /api/v1/custom-resources/:type/:id` - Delete a resource instance

## CORS Configuration

If you encounter CORS errors when accessing the APIs, you may need to configure CORS headers on the backend services. The Flask applications should include appropriate CORS configuration:

```python
from flask_cors import CORS

app = Flask(__name__)
CORS(app)  # Enable CORS for all routes
```

Or add the `flask-cors` package to requirements.txt and install it.

## Troubleshooting

### Cannot connect to APIs

1. Verify the services are running:
   ```bash
   curl http://localhost:11102/
   curl http://localhost:11101/
   ```

2. Check the URLs in the Configuration panel match your setup

3. Check browser console for detailed error messages

### CORS Errors

- Run the webapp through a web server (not file://)
- Ensure the backend APIs have CORS enabled
- Try using the browser's CORS extension for development

### Services JSON Validation Error

Make sure your services JSON is valid:
- Use proper JSON syntax (double quotes for strings)
- Include required fields: `service_name`, `image`
- Use JSON validators online to check your JSON

## Project Structure

```
webapp/
├── src/
│   ├── app/
│   │   ├── components/          # Feature components
│   │   │   ├── marketplace/
│   │   │   ├── installed-addons/
│   │   │   ├── hooks/
│   │   │   └── custom-resources/
│   │   ├── models/              # TypeScript interfaces
│   │   │   └── addon.model.ts
│   │   ├── services/            # API services
│   │   │   ├── marketplace.service.ts
│   │   │   ├── addons-engine.service.ts
│   │   │   ├── resource-abstractor.service.ts
│   │   │   └── config.service.ts
│   │   ├── app.component.ts     # Root component
│   │   ├── app.config.ts        # App configuration
│   │   └── app.routes.ts        # Routing (if needed)
│   ├── index.html               # HTML entry point
│   ├── main.ts                  # Angular bootstrap
│   └── styles.css               # Global styles
├── angular.json                 # Angular configuration
├── tsconfig.json                # TypeScript configuration
├── package.json                 # Dependencies
├── Dockerfile                   # Docker build
└── nginx.conf                   # nginx configuration

## Development Tips

### Using Angular DevTools

Install the Angular DevTools browser extension for debugging components and inspecting change detection.

### Hot Module Replacement

The development server includes HMR - changes are reflected instantly without full page reload.

### Type Safety

The application is fully typed with TypeScript. Run `npm run build` to check for type errors.

### Testing

```bash
npm test
```

## License

Part of the Oakestra project. See the main project LICENSE file.
