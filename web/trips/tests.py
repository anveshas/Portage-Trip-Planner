import os
import subprocess
import time
import urllib.request
from pathlib import Path

from django.conf import settings
from django.test import TestCase

from trips.bedrock import build_bedrock_body, extract_text
from trips.narrative import audit, narrate


class NarrativeTests(TestCase):
    def plan(self):
        return {
            "route": "Delhi → Jaipur → Delhi",
            "budget": 80000,
            "total": 18935,
            "fits_budget": True,
            "valid": True,
            "train_note": "Trains are not booked. Check IRCTC.",
            "rules": {"meal_per_traveler_night": 700, "local_per_traveler_city": 500},
            "legs": [
                {
                    "carrier": "RedBus",
                    "id": "RB101",
                    "from_name": "Delhi",
                    "to_name": "Jaipur",
                    "date": "2026-12-10",
                    "total": 1700,
                    "per_person": 850,
                    "source": "sample",
                }
            ],
            "stays": [
                {
                    "name": "Station Inn",
                    "city_name": "Jaipur",
                    "nights": 2,
                    "room_total": 3600,
                    "nightly": 1800,
                    "tax_rate": 12,
                    "tax": 432,
                    "source": "sample",
                }
            ],
            "lines": [
                {"kind": "meals", "label": "Meals, ₹700 per traveler for each night away", "amount": 2800, "source": "estimated"},
                {"kind": "local", "label": "Local transport, ₹500 per traveler in each city", "amount": 1000, "source": "estimated"},
                {"kind": "contingency", "label": "Contingency, 8% of the other booked costs", "amount": 1403, "source": "calculated"},
            ],
            "alternatives": [{"per_person": 450, "source": "manual"}],
        }

    def test_template_only_cites_plan_prices(self):
        plan = self.plan()
        text = narrate(plan)
        self.assertIn("Station Inn", text)
        self.assertIn("IRCTC", text)
        self.assertIn("The schedule checks passed.", text)
        self.assertTrue(audit(text, plan))
        self.assertFalse(audit(text + " A secret fare costs ₹99999.", plan))

    def test_missing_stays_still_narrates(self):
        plan = self.plan()
        plan["stays"] = None
        plan["fits_budget"] = False
        plan["valid"] = False
        text = narrate(plan)
        self.assertIn("No hotels were booked", text)
        self.assertIn("The schedule checks failed.", text)
        self.assertTrue(audit(text, plan))

    def test_nova_request_shape(self):
        body = build_bedrock_body("amazon.nova-lite-v1:0", "system", "user")
        self.assertEqual(body["messages"][0]["content"][0]["text"], "user")
        text = extract_text("amazon.nova-lite-v1:0", {"output": {"message": {"content": [{"text": "hello"}]}}})
        self.assertEqual(text, "hello")


class PlannerFlowTests(TestCase):
    @classmethod
    def setUpClass(cls):
        super().setUpClass()
        repo = Path(__file__).resolve().parents[2]
        binary = repo / "web" / ".gate-bin"
        subprocess.check_call(["go", "build", "-o", str(binary), "./cmd/gate"], cwd=repo / "gate")
        env = {**os.environ, "PORT": "18082"}
        env.pop("AMADEUS_CLIENT_ID", None)
        env.pop("AMADEUS_CLIENT_SECRET", None)
        cls.proc = subprocess.Popen(
            [str(binary)],
            env=env,
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
        )
        deadline = time.time() + 8
        while time.time() < deadline:
            try:
                with urllib.request.urlopen("http://127.0.0.1:18082/health", timeout=0.3) as response:
                    if response.status == 200:
                        break
            except Exception:
                time.sleep(0.05)
        else:
            cls.proc.kill()
            raise RuntimeError("gate failed to start")
        settings.GATE_URL = "http://127.0.0.1:18082"

    @classmethod
    def tearDownClass(cls):
        cls.proc.terminate()
        cls.proc.wait(timeout=5)
        super().tearDownClass()

    def test_page_plans_india_trip_inside_budget(self):
        response = self.client.post(
            "/",
            {
                "origin": "DEL",
                "stop1_city": "JAI",
                "stop1_nights": 2,
                "stop2_city": "BOM",
                "stop2_nights": 2,
                "start_date": "2026-12-10",
                "travelers": 2,
                "budget": 80000,
            },
        )
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, "Station Inn")
        self.assertContains(response, "Bandra Rooms")
        self.assertContains(response, "RB101")
        self.assertContains(response, "50540")
        self.assertContains(response, "fits the budget")
        self.assertContains(response, "The schedule checks passed.")
        self.assertContains(response, "IRCTC")
        self.assertContains(response, "not confirmed tickets")
        text = response.context["trip"].itinerary.narrative
        self.assertNotIn("₹99999", text)

    def test_tight_budget_still_shows_the_full_cost(self):
        response = self.client.post(
            "/",
            {
                "origin": "DEL",
                "stop1_city": "JAI",
                "stop1_nights": 2,
                "stop2_city": "BOM",
                "stop2_nights": 2,
                "start_date": "2026-12-10",
                "travelers": 2,
                "budget": 40000,
            },
        )
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, "fits")
        self.assertContains(response, "35891")
        self.assertContains(response, "50540")
        self.assertContains(response, "Bandra Rooms")
        self.assertContains(response, "not confirmed tickets")
        self.assertContains(response, "last city")
        self.assertNotContains(response, "Station Inn")

    def test_api_rejects_a_stop_in_the_origin_city(self):
        response = self.client.post(
            "/api/v1/plan",
            data='{"origin":"DEL","start_date":"2026-12-10","travelers":2,"budget":80000,"stops":[{"city":"DEL","nights":2}]}',
            content_type="application/json",
        )
        self.assertEqual(response.status_code, 400)

    def test_goa_explains_the_connection(self):
        response = self.client.post(
            "/",
            {
                "origin": "DEL",
                "stop1_city": "GOI",
                "stop1_nights": 2,
                "stop2_city": "",
                "stop2_nights": 2,
                "start_date": "2026-12-10",
                "travelers": 2,
                "budget": 80000,
            },
        )
        self.assertEqual(response.status_code, 200)
        self.assertContains(response, "Mumbai")
        self.assertContains(response, "6E102")
        self.assertContains(response, "6E330")
        self.assertContains(response, "another departure date")
        self.assertContains(response, "not confirmed tickets")
