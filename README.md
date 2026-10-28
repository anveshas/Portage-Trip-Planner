# Portage

A deterministic, budget-constrained travel optimization engine for a multi-city trip in India. Go decides the itinerary, the price, and whether the schedule is possible. The written explanation is assistance on top of that decision: a template, or Amazon Bedrock when configured. The prose can only mention amounts that are already in the plan. The full request path, rules, and commands are in [WORKFLOW.md](WORKFLOW.md).

The budget is one total for the whole group. A hotel rate is per room per night, with one room for every two travelers, rounded up. The cost order is transport, then the room total, then hotel tax on that room total only, then the meal allowance, then local transport, then an 8% contingency on that sum once. Contingency is not applied to itself, and a missing price is never treated as zero.

A connection is feasible only when the arrival instant, the next departure instant, and the transfer buffer all fit. The buffer is 90 minutes at the same station and 120 minutes when the station or mode changes. Sample clocks are stored with the India offset, and the check compares instants, so a time zone is not ignored. An overnight arrival is not booked.

When the requested trip is over budget, the engine keeps the best plan that still fits. Best means the most requested cities, then a plan that still includes the last requested city, then the most nights, then the lower cost. A longer stay in an earlier city does not beat the last city.

Every priced option stores its retrieval time, source, currency, data status, and expiry. Sample inventory is `sample`. An Amadeus test search is `live_test`: it is not commercial availability and it is not a guaranteed fare. An expired or unavailable quote is flagged and is not treated as bookable. A live quote can be searched again. A sample quote does not become live.

Trains are listed as an IRCTC check. They are not searched and they are not booked.

```text
Django  →  Go planner
             sample inventory, or Amadeus test flights when keys are set
             hotel, tax, meals, local transport, contingency
             transfer and night checks
         ←  priced plan, with a source on every line
Django writes the itinerary (template, or Amazon Bedrock)
```

Sources on a line mean:

- **live** — an Amadeus test search returned this fare on this request
- **sample** — the development inventory, used when live search is off or a search fails
- **estimated** — meals and local transport, from fixed rules
- **calculated** — hotel tax and the contingency
- **manual** — an IRCTC estimate that is shown and not booked

Without `AMADEUS_CLIENT_ID` and `AMADEUS_CLIENT_SECRET`, every flight and bus is sample data. Hotels are sample data either way. This is not a live booking site.

## Run it

Go 1.22+ and Python 3.12.

```bash
# terminal 1
cd gate
go run ./cmd/gate

# terminal 2
cd web
python3.12 -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
python manage.py migrate
python manage.py runserver
```

Open http://127.0.0.1:8000. The default trip is Delhi to Jaipur for two nights, then Mumbai for two nights, two travelers, ₹80,000, leaving 2026-12-10. The sample plan books the RedBus to Jaipur, IndiGo onward, Station Inn and Bandra Rooms, and totals ₹50,540.

```bash
curl -s -X POST http://127.0.0.1:8000/api/v1/plan \
  -H 'content-type: application/json' \
  -d '{"origin":"DEL","start_date":"2026-12-10","travelers":2,"budget":80000,"stops":[{"city":"JAI","nights":2},{"city":"BOM","nights":2}]}'
```

A budget that cannot hold the requested trip is searched again. At ₹40,000 the Delhi, Jaipur, Mumbai request cannot keep both cities. The engine keeps Mumbai for one night at ₹35,891, because Mumbai is the last city in the request. Jaipur for two nights is cheaper and is listed among the cuts. If nothing fits, the reply is the cheapest version, the cost of the trip you asked for, and the other cuts. `GET /health` reports `self_check: passed` and `self_check_total: 50540` only when the ₹80,000 sample trip still builds. It also counts plans, API failures, and the last request error.

The reply includes `plan.report` and `plan.rules`. Every ride names its provider, currency, data status, retrieval time, and expiry. A sample fare is labeled as not a confirmed ticket. An Amadeus test fare is labeled as not commercial availability.

To try a real flight search, put Amadeus **test** credentials in the environment of the gate process (`AMADEUS_HOST` defaults to `https://test.api.amadeus.com`). There is no key in this repo, so the running gate stays on the sample inventory until you set `AMADEUS_CLIENT_ID` and `AMADEUS_CLIENT_SECRET`. Offers in INR are kept. A fare in another currency is converted only when `FX_USD_INR` is a positive rate, and a fractional rupee rounds up. A timeout, an empty result, or a currency with no rate leaves that leg on the sample inventory. Failures are logged and counted on `/health`.

Set `MODEL_PROVIDER=bedrock` when you want Nova to write the prose. If that draft mentions a price that is not in the plan, Portage keeps the template instead. On EKS the web pod uses the IAM policy in `infra/bedrock-policy.json` through the `portage-web` service account. No API key goes in the image. Add the Amadeus test credentials to the gate pod yourself. They are not in `k8s/portage.yaml`.

## Kubernetes

`k8s/portage.yaml` runs both services. The gate is only reachable inside the cluster. Replace `ACCOUNT_ID`, push the images from the Dockerfiles to ECR, and create the secret the web pod reads:

```bash
kubectl apply -f k8s/portage.yaml
kubectl -n portage create secret generic portage \
  --from-literal=django-secret=change-me \
  --from-literal=database-url=postgres://user:pass@host:5432/portage
```

`docker-compose.yml` is the same two containers plus Postgres, without a cluster.

## Tests

```bash
cd gate && go test ./...
cd web && python manage.py test
```

The planner tests check that the total equals the line items, hotel nights cover the stays, every connection meets the timing rule, the budget is respected when a feasible option exists, tax and contingency are applied once, the last requested city outranks a longer earlier stay, and an expired live quote is flagged and searched again. The flight normalizer drops a non-INR fare and a fare with no price. The web test rejects a narration that cites a price the plan does not contain.
