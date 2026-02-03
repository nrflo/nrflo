# nrworkflow UI

Web interface for managing nrworkflow tickets and workflows.

## Prerequisites

- Node.js 18+
- nrworkflow Go binary (`~/.nrworkflow/nrworkflow/`)

## Quick Start

```bash
# 1. Start both API server and UI
./start-server.sh

# Or manually:
# Start API server
nrworkflow serve

# Start UI dev server (in another terminal)
npm run dev

# 3. Open http://localhost:5173
```

## Architecture

```
┌─────────────────┐     HTTP/JSON      ┌──────────────────┐     SQLite     ┌─────────────────┐
│  React UI       │ ◄────────────────► │ nrworkflow serve │ ◄────────────► │ nrworkflow.data │
│  (localhost:    │    X-Project       │ (localhost:6587) │                │ (~/.nrworkflow/)│
│   5173)         │    header          │                  │                │                 │
└─────────────────┘                    └──────────────────┘                └─────────────────┘
```

## Development

### Install Dependencies

```bash
npm install
```

### Start Dev Server

```bash
npm run dev
```

The Vite dev server runs on port 5173 and proxies `/api` requests to the backend at port 6587.

### Build for Production

```bash
npm run build
```

Output is in the `dist/` directory.

### Type Check

```bash
npm run typecheck
```

## Configuration

### API URL

By default, the UI uses the Vite proxy to reach the API. For production or custom setups, create a `.env` file:

```bash
cp .env.example .env
# Edit .env if needed
```

### Multi-Project Support

The UI supports multiple projects via the project selector in the header. Projects are loaded from the `/api/v1/projects` endpoint.

Create projects using the CLI:
```bash
nrworkflow project create myproject --name "My Project"
```

Server configuration is in `~/.nrworkflow/config.json`:
```json
{
  "server": {
    "port": 6587,
    "cors_origins": ["http://localhost:5173"]
  }
}
```

## Features

### Dashboard
- Overview of ticket counts by status
- Active (pending/in-progress) tickets
- Recently closed tickets
- Quick actions

### Ticket List
- Filter by status and type
- Full-text search
- Create new tickets

### Ticket Detail
- View full ticket details
- Dependencies (blockers/blocks)
- Workflow phase timeline
- Close/delete tickets

### Workflow Visualization
- Phase timeline with status indicators
- Active agent display (PID, session)
- Expandable findings per phase
- Agent history

## Tech Stack

- **Vite** - Build tool
- **React 18** - UI framework
- **TypeScript** - Type safety
- **TanStack Query** - Server state management
- **Zustand** - Client state (project selection)
- **Tailwind CSS v4** - Styling
- **React Router v6** - Routing
- **react-hook-form + zod** - Form handling

## Project Structure

```
src/
├── api/           # API client and endpoints
├── components/
│   ├── layout/    # Header, Sidebar, Layout
│   ├── tickets/   # TicketCard, TicketList, TicketForm
│   ├── ui/        # Badge, Button, Card, Input, etc.
│   └── workflow/  # PhaseTimeline, PhaseCard, FindingsViewer
├── hooks/         # React Query hooks
├── lib/           # Utilities
├── pages/         # Route pages
├── stores/        # Zustand stores
└── types/         # TypeScript types
```

## API Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/api/v1/projects` | List projects |
| `GET` | `/api/v1/projects/:id` | Get project |
| `POST` | `/api/v1/projects` | Create project |
| `DELETE` | `/api/v1/projects/:id` | Delete project |
| `GET` | `/api/v1/tickets` | List tickets (filter: status, type) |
| `GET` | `/api/v1/tickets/:id` | Get single ticket with deps |
| `POST` | `/api/v1/tickets` | Create ticket |
| `PATCH` | `/api/v1/tickets/:id` | Update ticket |
| `POST` | `/api/v1/tickets/:id/close` | Close ticket |
| `DELETE` | `/api/v1/tickets/:id` | Delete ticket |
| `GET` | `/api/v1/tickets/:id/workflow` | Get parsed workflow state |
| `PATCH` | `/api/v1/tickets/:id/workflow` | Update workflow state |
| `GET` | `/api/v1/tickets/:id/agents` | Get agent sessions |
| `GET` | `/api/v1/search?q=` | FTS5 search |
| `POST` | `/api/v1/dependencies` | Add dependency |
| `DELETE` | `/api/v1/dependencies` | Remove dependency |
| `GET` | `/api/v1/status` | Dashboard summary |

All ticket endpoints require `X-Project` header to select the project.
