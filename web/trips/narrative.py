import re

MONEY = re.compile(r"₹(\d+(?:,\d{3})*(?:\.\d{2})?)")


def money_amounts(plan):
    amounts = {plan["budget"], plan["total"], plan.get("requested_total") or 0, plan.get("minimum_budget") or 0}
    rules = plan.get("rules") or {}
    for key in ("meal_per_traveler_night", "local_per_traveler_city"):
        if rules.get(key) is not None:
            amounts.add(rules[key])
    for leg in plan.get("legs") or []:
        amounts.add(leg.get("total") or 0)
        amounts.add(leg.get("per_person") or 0)
    for stay in plan.get("stays") or []:
        amounts.add(stay.get("nightly") or 0)
        amounts.add(stay.get("room_total") or 0)
        amounts.add(stay.get("tax") or 0)
    for line in plan.get("lines") or []:
        amounts.add(line.get("amount") or 0)
    for alt in plan.get("alternatives") or []:
        amounts.add(alt.get("per_person") or 0)
    for cut in plan.get("reductions") or []:
        amounts.add(cut.get("total") or 0)
    recovery = plan.get("recovery") or {}
    for suggestion in recovery.get("suggestions") or []:
        amounts.add(suggestion.get("total") or 0)
    return {str(amount) for amount in amounts}


def audit(narrative, plan):
    allowed = money_amounts(plan)
    for raw in MONEY.findall(narrative):
        if raw.replace(",", "") not in allowed:
            return False
    return True


def narrate(plan):
    if plan.get("booked") is False:
        recovery = plan.get("recovery") or {}
        parts = [plan.get("adjustment") or recovery.get("reason") or "No ride was booked."]
        if recovery.get("date_note") and recovery.get("date_note") not in parts[0]:
            parts.append(recovery["date_note"])
        for suggestion in recovery.get("suggestions") or []:
            if suggestion.get("total"):
                parts.append(f"{suggestion['label']}: ₹{suggestion['total']:,}. {suggestion.get('detail') or ''}")
            else:
                parts.append(f"{suggestion['label']}. {suggestion.get('detail') or ''}")
        if plan.get("fare_notice"):
            parts.append(plan["fare_notice"])
        text = " ".join(part for part in parts if part)
        if not audit(text, plan):
            raise RuntimeError("template narrative cited a price that is not in the plan")
        return text
    parts = []
    if plan.get("adjustment"):
        parts.append(plan["adjustment"])
    parts.append(f"{plan.get('route', 'This trip')}.")
    for leg in plan.get("legs") or []:
        parts.append(
            f"{leg['carrier']} {leg['id']} {leg['from_name']} to {leg['to_name']} on {leg['date']}, "
            f"₹{leg['total']:,} ({leg.get('quote') or leg['source']})."
        )
    if not plan.get("stays"):
        parts.append("No hotels were booked.")
    for stay in plan.get("stays") or []:
        parts.append(
            f"Stay at {stay['name']} in {stay['city_name']} for {stay['nights']} nights, "
            f"₹{stay['room_total']:,} before tax ({stay['source']}). Tax {stay['tax_rate']}% is ₹{stay['tax']:,}."
        )
    for line in plan.get("lines") or []:
        if line["kind"] in {"meals", "local", "contingency"}:
            parts.append(f"{line['label']}: ₹{line['amount']:,} ({line['source']}).")
    parts.append(plan.get("train_note") or "Trains are not booked. Check IRCTC.")
    status = "inside" if plan.get("fits_budget") else "over"
    parts.append(f"Total ₹{plan['total']:,} is {status} the ₹{plan['budget']:,} budget.")
    if plan.get("valid"):
        parts.append("The schedule checks passed.")
    else:
        parts.append("The schedule checks failed.")
    if plan.get("fare_notice"):
        parts.append(plan["fare_notice"])
    text = " ".join(parts)
    if not audit(text, plan):
        raise RuntimeError("template narrative cited a price that is not in the plan")
    return text
