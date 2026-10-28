# Portage workflow

This is the operating guide for the project. The README is the short description. This file is the path a request takes, the rules the planner applies, and how to run and check it.

## What the two processes do

Portage is two programs.

| Process | Where | Port | Job |
| --- | --- | --- | --- |
| Django | `web/` | 8000 | Form, JSON API, saving the trip, writing the itinerary |
| Go gate | `gate/` | 8080 | Routes, prices, transfer checks, budget ranking, quote status |

Django does not calculate the fare, the tax, or whether a connection is possible. It sends the request to the gate and stores the JSON that comes back. The itinerary text is written only after that JSON exists.

Cities in the inventory: Delhi `DEL`, Jaipur `JAI`, Mumbai `BOM`, Goa `GOI`.

## What happens when you click Plan the trip

1. The browser posts the form to `POST /` in `web/trips/views.py`.
2. Django checks the form: at least one city, at most two, total nights from 1 to 7, a real date.
3. Django posts JSON to `GATE_URL/v1/plan`. The default URL is `http://127.0.0.1:8080/v1/plan`.
4. The gate checks the request again, builds the trip, and returns one plan.
5. Django writes the narrative from that plan. With `MODEL_PROVIDER=template`, the text is a fixed template. With `bedrock`, Amazon Nova drafts it, and a price audit rejects the draft if it cites a rupee the plan does not contain.
6. Django saves a `Trip` and an `Itinerary` (the plan JSON plus the narrative) and renders the page.

`POST /api/v1/plan` does the same work and returns JSON: `trip_id`, `narrative`, `plan`, and `report`.

A request body looks like this:

```json
{
  "origin": "DEL",
  "start_date": "2026-12-10",
  "travelers": 2,
  "budget": 80000,
  "stops": [
    {"city": "JAI", "nights": 2},
    {"city": "BOM", "nights": 2}
  ]
}
```

The gate rejects a request when any of these is true:

- travelers are outside 1 to 4
- the budget is not greater than zero
- there is not one or two stops
- a city code is unknown
- two stops in a row are the same city
- a stay is outside 1 to 4 nights
- the stays add up to more than 7 nights

## How one exact trip is built

`Build` in `gate/internal/plan/replan.go` calls `buildExact` for the trip you asked for.

For each hop, including the ride home, `travel` in `route.go` does this:

1. Look for a direct same-day flight or bus.
2. If none is bookable, try one same-day connection through another inventory city. Delhi to Goa has no direct ride, so the sample plan uses Delhi to Mumbai (`6E102`) and then Mumbai to Goa (`6E330`).
3. If that also fails, the trip is not booked. The reply explains the missing hop. Sample fares do not change by date. A nearby date is searched only when Amadeus is enabled.

A ride is skipped, and listed under "Not booked", when:

- its price is 0 (a missing price is never treated as free)
- it arrives the next day (an overnight ride would add a hotel night the stay does not include)
- it is a train (shown as an IRCTC estimate, source `manual`, and not added to the total)

Among the rides that remain, the gate picks the cheapest one whose departure is far enough after the previous arrival.

Then it picks the cheapest hotel in that city. Rooms are one per two travelers, rounded up: 1 or 2 travelers use 1 room, 3 or 4 use 2. The nightly rate is per room.

## How the money is added

Currency is INR. The budget is one total for the whole group, not an amount per person.

Sample transport prices in the inventory are per person. The line amount is that price times the number of travelers. An Amadeus `grandTotal` is already the group total, and a fractional rupee rounds up.

For each hotel:

- room total = nightly rate × nights × rooms
- tax is 12% of the room total when the nightly rate is ₹7,500 or less, otherwise 18%
- tax is rounded half up: `(amount × percent + 50) / 100`
- tax is not applied to meals, local transport, or fares

Then:

- meals = ₹700 × travelers × nights away
- local transport = ₹500 × travelers × number of cities stayed in
- subtotal = transport + room totals + hotel tax + meals + local transport
- contingency = 8% of that subtotal, rounded half up, added once
- total = subtotal + contingency

Contingency is not applied to itself.

The default trip, Delhi, Jaipur 2 nights, Mumbai 2 nights, 2 travelers, leaving 2026-12-10, totals ₹50,540 against an ₹80,000 budget. The booked pieces are RedBus `RB101`, IndiGo `6E440`, IndiGo `6E880`, Station Inn, and Bandra Rooms, plus meals, local transport, tax, and contingency.

## What happens when the trip is over budget

If the exact trip is valid but over budget, the gate builds cheaper shapes:

- fewer nights in the same cities, down to 1 night each
- each city alone, down to 1 night

The exact request is skipped in that search. Each shape is priced with the same rules.

The best shape that is valid and inside the budget wins, in this order:

1. most of the cities you asked for
2. a plan that still includes the last city you asked for
3. most nights
4. lower cost

At ₹40,000 the two-city trip cannot be kept. Both cities for 1 night each is ₹40,984. The winner is Mumbai for 1 night at ₹35,891, because Mumbai is the last city. Jaipur for 2 nights is ₹18,935 and is listed as a cheaper cut, not the chosen plan.

If no shape fits, the reply shows the cheapest version, the cost of the trip you asked for, and every cut it tried. `fits_budget` is false.

## What the schedule check rejects

`Validate` runs before a plan is called valid. It fails when:

- booked line items do not add up to `total`
- a booked line has an amount of 0 or less
- a train is marked as anything other than `manual`
- `fits_budget` does not match `total <= budget`
- a hotel's check-in and check-out do not span the stated nights
- hotel stays overlap or leave a gap
- the path does not start at the origin, or a leg does not continue from the previous leg
- no ride arrives in a stay city on the check-in date
- the ride home does not leave the last city on the checkout date and return to the origin
- a ride's arrival is not after its own departure
- the gap between one arrival and the next departure is too short
- a quote status is `expired` or `unavailable`

Transfer buffers, compared as real instants:

- 90 minutes when the arrival station and the next departure station are the same
- 120 minutes when the station or the mode changes

Sample clocks are stored with the India offset `+05:30`. A time written in another offset is not given extra slack. `18:00+05:30` and `13:00Z` are 30 minutes apart.

## What each price label means

Every priced ride, hotel, and line stores currency (`INR`), provider, retrieval time, expiry, data status, and a quote sentence.

| Source | Data status | Meaning | Expiry |
| --- | --- | --- | --- |
| `sample` | `sample` | Development inventory. Not a confirmed ticket or reservation. | 24 hours |
| `live` | `live_test` | An Amadeus test search on this request. Not commercial availability and not a guaranteed fare. | 15 minutes |
| `estimated` | `estimated` | Meals and local transport, from the fixed rules above. | none |
| `calculated` | `calculated` | Hotel tax and the contingency. | none |
| `manual` | `manual` | An IRCTC estimate. Shown and not booked. | none |

`expired` and `unavailable` are not bookable. `Revalidate` searches again only for an expired live leg. An empty new search becomes `unavailable` and keeps the old amount so the ride is not treated as free. A sample quote is never turned into a live quote.

Without `AMADEUS_CLIENT_ID` and `AMADEUS_CLIENT_SECRET`, the gate does not call Amadeus. Hotels stay on sample rates even when flight search is on. A non-INR Amadeus fare is kept only when `FX_USD_INR` is a positive rate. Otherwise that fare is skipped and the sample ride remains.

## What the page is allowed to say

`web/trips/narrative.py` builds the paragraph from the route, the legs, the hotels, meals, local transport, contingency, the train note, and whether the total fits. `audit` scans every `₹` amount in that text. If a number is not already on the plan, the budget, the rules, a reduction, or a recovery suggestion, the narrative is rejected.

Bedrock uses the same audit. A failing draft is discarded and the template is saved instead. `narrative_source` on the itinerary is `template` or `bedrock`.

## Health

`GET http://127.0.0.1:8080/health` builds the default sample trip with flight search turned off. It reports `self_check: passed` and `self_check_total: 50540` only when that trip is still valid, inside ₹80,000, and totals ₹50,540. An Amadeus key cannot change that number, because the check does not call Amadeus. The same body counts plans, API failures, and the last request error.

`GET http://127.0.0.1:8000/health` only says the Django process is up. `GET http://127.0.0.1:8000/health/ready` is ok only when the gate health check passes.

## Run it on this machine

```bash
export PATH="/opt/homebrew/bin:$PATH"

# terminal 1
cd gate
go run ./cmd/gate

# terminal 2
cd web
../.venv/bin/python manage.py migrate
../.venv/bin/python manage.py runserver 127.0.0.1:8000
```

Open http://127.0.0.1:8000. Leave the Amadeus fields empty unless you have test credentials. The template writer needs no AWS account.

Checks:

```bash
curl -s http://127.0.0.1:8080/health
curl -s http://127.0.0.1:8000/health/ready

cd gate && go test ./...
cd web && ../.venv/bin/python manage.py test
```

The Django tests compile their own gate and listen on port 18082. They do not use the gate you started on 8080.

Things worth clicking:

- Default form: ₹50,540, `RB101`, Station Inn, Bandra Rooms, and the sample-fare notice.
- Budget `40000`: Mumbai for 1 night at ₹35,891, with Jaipur for 2 nights listed as a cheaper cut.
- First city Goa, no second city, budget `80000`: Delhi–Mumbai–Goa the same day on `6E102` and `6E330`.

## Where the code lives

| Path | What it owns |
| --- | --- |
| `web/trips/views.py` | Form, API, save |
| `web/trips/gate.py` | HTTP call to the gate and the ready check |
| `web/trips/narrative.py` | Template text and the price audit |
| `web/trips/bedrock.py` | Optional Nova draft |
| `web/trips/templates/trips/home.html` | The page |
| `web/trips/models.py` | `Trip` and `Itinerary` |
| `gate/cmd/gate/main.go` | Process entry, port 8080 |
| `gate/internal/server/server.go` | `/health` and `/v1/plan` |
| `gate/internal/plan/plan.go` | One exact trip and the money lines |
| `gate/internal/plan/route.go` | Direct rides and same-day connections |
| `gate/internal/plan/replan.go` | Over-budget search and ranking |
| `gate/internal/plan/validate.go` | Schedule and total checks |
| `gate/internal/plan/provenance.go` | Quote text, currency, status, expiry |
| `gate/internal/plan/freshness.go` | Expired quotes and revalidation |
| `gate/internal/catalog/inventory.json` | Sample flights, buses, hotels, train estimates |
| `gate/internal/provider/amadeus.go` | Optional Amadeus test client |
| `docker-compose.yml` | Postgres, gate, and web |
| `k8s/portage.yaml` | Cluster manifest. It has not been applied. |

Local Django uses SQLite. Docker Compose uses PostgreSQL.

## What this project is not

It does not book tickets. It does not read MakeMyTrip, Agoda, Ixigo, IRCTC, or RedBus. Sample inventory and an Amadeus test search are not live fares you can buy. The Kubernetes file is a deploy description, not a running cluster.
