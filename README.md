# Court-Data Fetcher & Mini-Dashboard

A web application to fetch and display case information from Delhi District Courts (e-Courts system).

## Court Chosen
**Delhi District Courts** - https://districts.ecourts.gov.in/delhi

This court was chosen because:
- It has a standardized e-Courts interface
- Provides comprehensive case information
- Has predictable URL patterns and form structures

## Features

### Core Features
- Web form for case search (Case Type, Case Number, Filing Year)
- Web scraping using Rod (headless browser automation)
- SQLite database for storing queries and results
- Clean UI with Bootstrap for displaying case details
- PDF download links for orders/judgments
- Pagination support for multiple orders
- Comprehensive error handling
- Environment-based configuration

### Bonus Features Implemented
- **Concurrency**: Parallel fetching using Go routines and channels
- **Caching**: In-memory LRU cache to reduce repeated scrapes
- **REST API**: JSON endpoints at `/api/case` and `/api/cases`
- **Structured Logging**: Using Zap logger with configurable levels
- **Docker Support**: Full containerization with docker-compose
- **CI/CD**: GitHub Actions workflow for testing and building

## CAPTCHA Handling Strategy

The application implements a comprehensive multi-strategy approach for CAPTCHA handling:

1. **Automated CAPTCHA Services** (Primary Method):
   - **2Captcha Integration**: Set `TWOCAPTCHA_API_KEY` in `.env`
   - **Anti-Captcha Integration**: Set `ANTICAPTCHA_API_KEY` in `.env`
   - Services solve CAPTCHAs automatically within 10-30 seconds

2. **Manual Browser Input** (Development/Testing):
   - Set `HEADLESS_MODE=false` to see the browser
   - Enter CAPTCHA directly in the browser when prompted
   - Useful for development and testing

3. **Remote Manual Solving** (Fallback):
   - CAPTCHA images are saved and accessible via API
   - Access `/captcha?id=CAPTCHA_ID` to solve manually
   - Submit solution via web interface or API

4. **Session Persistence**:
   - Maintains browser cookies and session state
   - Reduces CAPTCHA frequency for subsequent requests
   - Automatic retry with new CAPTCHA on failure

## Setup CAPTCHA Services

### Option 1: 2Captcha (Recommended)
1. Sign up at https://2captcha.com
2. Get your API key
3. Add to `.env`: `TWOCAPTCHA_API_KEY=your_key_here`
4. Top up your account (usually $1-3 per 1000 CAPTCHAs)

### Option 2: Anti-Captcha
1. Sign up at https://anti-captcha.com
2. Get your API key
3. Add to `.env`: `ANTICAPTCHA_API_KEY=your_key_here`

### Option 3: Manual Solving Endpoint
Access `/captcha?id=CAPTCHA_ID` to manually solve CAPTCHAs through the web interface.

## Setup Instructions

### Prerequisites
- Go 1.21 or higher
- Docker and Docker Compose (optional)
- Git

### Environment Variables

Create a `.env` file based on `.env.example`:

```bash
cp .env.example .env
```

Required variables:
- `PORT`: Server port (default: 8080)
- `DATABASE_PATH`: SQLite database path
- `LOG_LEVEL`: Logging level (debug, info, warn, error)
- `CACHE_SIZE`: LRU cache size
- `CACHE_TTL`: Cache TTL in minutes
- `COURT_BASE_URL`: Base URL for the court website
- `SCRAPER_TIMEOUT`: Scraper timeout in seconds
- `HEADLESS_MODE`: Run browser in headless mode (true/false)

### Running with Docker (Recommended)

1. Build and run with docker-compose:
```bash
docker-compose up --build
```

2. Access the application at http://localhost:8080

### Running Locally

1. Install dependencies:
```bash
go mod download
```

2. Run database migrations:
```bash
go run cmd/server/main.go migrate
```

3. Start the server:
```bash
go run cmd/server/main.go
```

### Running Tests

```bash
make test
```

### Building for Production

```bash
make build
```

## API Documentation

### Web Endpoints

- `GET /` - Home page with search form
- `POST /search` - Submit case search
- `GET /results/:id` - View search results

### REST API Endpoints

- `GET /api/case?type=CS&number=1234&year=2023` - Get case details
- `GET /api/cases` - List all cached cases
- `GET /api/health` - Health check endpoint

### Example API Response

```json
{
  "success": true,
  "data": {
    "case_number": "CS/1234/2023",
    "parties": {
      "petitioner": "John Doe",
      "respondent": "Jane Smith"
    },
    "filing_date": "2023-01-15",
    "next_hearing": "2024-02-20",
    "status": "Pending",
    "orders": [
      {
        "date": "2023-12-15",
        "description": "Order on IA",
        "pdf_link": "https://..."
      }
    ]
  }
}
```

## Architecture

### Technology Stack
- **Backend**: Go with Gin framework
- **Scraping**: Rod (headless Chrome automation)
- **Database**: SQLite with GORM
- **Frontend**: Server-side rendered HTML with Bootstrap
- **Caching**: In-memory LRU cache
- **Logging**: Zap structured logger
- **Container**: Docker with multi-stage build

### Key Components

1. **Scraper Module** (`internal/scraper/`)
   - Handles web scraping with Rod
   - CAPTCHA detection and handling
   - Session management
   - Concurrent scraping support

2. **API Module** (`internal/api/`)
   - HTTP handlers for web and REST endpoints
   - Request validation
   - Response formatting

3. **Cache Module** (`internal/cache/`)
   - LRU cache implementation
   - TTL support
   - Thread-safe operations

4. **Database Module** (`internal/database/`)
   - GORM models
   - Migration management
   - Query logging

## Robustness Features

1. **Graceful Degradation**: Falls back to cached data when scraping fails
2. **Retry Logic**: Automatic retry with exponential backoff
3. **Session Persistence**: Maintains browser session for multiple requests
4. **Error Recovery**: Comprehensive error handling at all levels
5. **Configurable Timeouts**: Prevents hanging on slow responses

## Legal Considerations

This application is designed for legitimate use cases such as:
- Legal professionals tracking their cases
- Litigants checking case status
- Academic research

Users must comply with the court website's terms of service and robots.txt.

## Contributing

1. Fork the repository
2. Create a feature branch
3. Commit your changes
4. Push to the branch
5. Create a Pull Request

## License

MIT License - see LICENSE file for details