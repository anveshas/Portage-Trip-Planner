import json
from datetime import datetime, timedelta

from django.conf import settings
from django.http import JsonResponse
from django.shortcuts import render
from django.views.decorators.csrf import csrf_exempt
from django.views.decorators.http import require_http_methods

from trips.bedrock import narrate_with_bedrock
from trips.gate import healthy, plan_trip
from trips.models import Itinerary, Trip
from trips.narrative import narrate

CITIES = [
    ("DEL", "Delhi"),
    ("JAI", "Jaipur"),
    ("BOM", "Mumbai"),
    ("GOI", "Goa"),
]

DEFAULTS = {
    "origin": "DEL",
    "stop1_city": "JAI",
    "stop1_nights": "2",
    "stop2_city": "BOM",
    "stop2_nights": "2",
    "start_date": "2026-12-10",
    "travelers": "2",
    "budget": "80000",
}


def _form(data):
    form = dict(DEFAULTS)
    if not data:
        return form
    for key in form:
        if key in data:
            form[key] = data.get(key)
    return form


def _stops(data):
    if not hasattr(data, "getlist") and data.get("stops"):
        stops = [{"city": stop["city"], "nights": int(stop["nights"])} for stop in data["stops"]]
    else:
        stops = []
        for prefix in ("stop1", "stop2"):
            city = (data.get(f"{prefix}_city") or "").strip()
            if not city:
                continue
            stops.append({"city": city, "nights": int(data.get(f"{prefix}_nights"))})
    if not stops:
        raise ValueError("Add at least one city.")
    if len(stops) > 2:
        raise ValueError("Plan at most two cities.")
    return stops


def _payload(data):
    start = datetime.strptime(data["start_date"], "%Y-%m-%d").date()
    stops = _stops(data)
    nights = sum(stop["nights"] for stop in stops)
    if nights < 1 or nights > 7:
        raise ValueError("Choose a trip of 1 to 7 nights.")
    end = start + timedelta(days=nights)
    return {
        "origin": data["origin"],
        "start_date": start.isoformat(),
        "travelers": int(data["travelers"]),
        "budget": int(data["budget"]),
        "stops": stops,
    }, start, end


def _save(payload, start, end, plan):
    if settings.MODEL_PROVIDER == "bedrock":
        narrative, source = narrate_with_bedrock(plan)
    else:
        narrative, source = narrate(plan), "template"
    trip = Trip.objects.create(
        origin=payload["origin"],
        destination=payload["stops"][-1]["city"],
        start_date=start,
        end_date=end,
        travelers=payload["travelers"],
        budget=payload["budget"],
        pace="budget",
        interests=[],
        stops=payload["stops"],
    )
    Itinerary.objects.create(trip=trip, plan=plan, narrative=narrative, narrative_source=source)
    return trip


@require_http_methods(["GET", "POST"])
def home(request):
    error = ""
    trip = None
    form = _form(request.POST if request.method == "POST" else None)
    if request.method == "POST":
        try:
            payload, start, end = _payload(request.POST)
            plan = plan_trip(payload)
            trip = _save(payload, start, end, plan)
        except (ValueError, RuntimeError, KeyError, TypeError) as exc:
            error = str(exc)
    return render(
        request,
        "trips/home.html",
        {"error": error, "trip": trip, "cities": CITIES, "form": form},
    )


@csrf_exempt
@require_http_methods(["POST"])
def plan_api(request):
    try:
        body = json.loads(request.body.decode() or "{}")
        payload, start, end = _payload(body)
        plan = plan_trip(payload)
        trip = _save(payload, start, end, plan)
    except (ValueError, RuntimeError, KeyError, TypeError, json.JSONDecodeError) as exc:
        return JsonResponse({"error": str(exc)}, status=400)
    return JsonResponse(
        {
            "trip_id": trip.id,
            "narrative": trip.itinerary.narrative,
            "plan": trip.itinerary.plan,
            "report": trip.itinerary.plan.get("report"),
        }
    )


def health(_request):
    return JsonResponse({"status": "ok"})


def ready(_request):
    try:
        if healthy():
            return JsonResponse({"status": "ok", "gate": "ok"})
    except Exception as exc:
        return JsonResponse({"status": "degraded", "gate": str(exc)}, status=503)
    return JsonResponse({"status": "degraded"}, status=503)
