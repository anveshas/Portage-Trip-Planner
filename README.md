
# Portage — Multi-City Trip Planner

Portage is a travel-planning web application that builds multi-city itineraries around a group's budget. It compares options from its inventory, checks travel connections, estimates accommodation and daily expenses, and adjusts the itinerary when the original plan exceeds the budget.

The project uses **Go** for itinerary planning and cost calculations, **Django** for the web application and API, and **Amazon Bedrock** for optional AI-generated trip explanations.

## Features

- **Budget-aware planning:** Calculates transport, hotel rooms, hotel taxes, meals, local transport, and contingency for the entire group.
- **Connection validation:** Checks departure and arrival times, transfer buffers, and overnight travel constraints.
- **Alternative itineraries:** Evaluates shorter stays and reduced-city options when the requested trip exceeds the budget.
- **Price transparency:** Tracks the source and status of quotes so sample prices aren't presented as confirmed bookings.
- **AI-assisted explanations:** Optionally uses Amazon Bedrock to generate an itinerary summary, with a template fallback if the generated text fails price validation.
- **Automated tests:** Includes Go planner tests and Django application tests.

## How It Works

1. Django receives a trip request through the web interface or API.
2. The Go planning service builds an itinerary, validates connections, and calculates the total cost.
3. If the requested trip exceeds the budget, the planner evaluates alternatives using defined ranking rules.
4. Django saves the resulting plan and generates an explanation.
5. Amazon Bedrock can optionally generate the explanation. Without AWS configuration, Portage uses a template.

## Technology Stack

- **Backend:** Python, Django, Go
- **AI Integration:** Amazon Bedrock
- **Database:** SQLite for local development, PostgreSQL with Docker Compose
- **Infrastructure:** Docker, Docker Compose, Kubernetes
- **Testing:** Go tests, Django tests

## Getting Started

### Prerequisites

Install the following before running the application:

- **Go 1.22 or later**
- **Python 3.12**
- **Git**

### 1. Clone the Repository

```bash
git clone https://github.com/anveshas/Portage-Trip-Planner.git
cd Portage-Trip-Planner
```

### 2. Set Up the Python Environment

From the project root, run:

```bash
python3.12 -m venv .venv
source .venv/bin/activate
pip install -r web/requirements.txt
```

### 3. Start the Go Planning Service

Open a terminal in the project root and run:

```bash
cd gate
go run ./cmd/gate
```

The Go service runs on port **8080** by default. Keep this terminal running.

### 4. Start the Django Application

Open a second terminal in the project root:

```bash
source .venv/bin/activate
cd web
python manage.py migrate
python manage.py runserver 127.0.0.1:8000
```

### 5. Open the Application

Visit the following URL in your browser:

**http://127.0.0.1:8000**

The default example plans a trip from **Delhi to Jaipur and then Mumbai** for two travelers, with a total budget of **₹80,000**.

### 6. Check Service Health

**Check the Go planning service:**

```bash
curl -s http://127.0.0.1:8080/health
```

**Check Django and its connection to the Go service:**

```bash
curl -s http://127.0.0.1:8000/health/ready
```

## Running Tests

### Go Tests

From the project root:

```bash
cd gate
go test ./...
```

### Django Tests

Open a separate terminal in the project root:

```bash
source .venv/bin/activate
cd web
python manage.py test
```

## Optional Integrations

### Amadeus

Without `AMADEUS_CLIENT_ID` and `AMADEUS_CLIENT_SECRET`, the planner uses sample transport inventory.

When configured, Amadeus test-environment searches can provide flight quotes, but these are not guaranteed commercial fares.

### Amazon Bedrock

The template-based explanation works without AWS credentials.

To enable Bedrock-generated explanations, configure the required AWS permissions and set:

```bash
export MODEL_PROVIDER=bedrock
```

## Data and Booking Limitations

Portage is a **planning prototype, not a ticket-booking service**.

- Built-in transport fares and hotel rates are sample data.
- Amadeus integration uses test-environment searches, not guaranteed bookable fares.
- Hotel prices remain sample data, even when flight search is enabled.
- Train options are informational estimates and are not searched or booked.
- The generated itinerary is a planning estimate, not a confirmed reservation.

## Project Documentation

See [`WORKFLOW.md`](WORKFLOW.md) for more details about:

- Architecture and request flow
- Pricing and budget calculations
- Connection validation and itinerary ranking
- API examples and health checks
- Testing and deployment with Docker or Kubernetes
