import json

from django.conf import settings

from trips.narrative import MONEY, audit, narrate


def build_bedrock_body(model_id, system, user):
    if model_id.startswith("amazon.nova"):
        return {
            "schemaVersion": "messages-v1",
            "system": [{"text": system}],
            "messages": [{"role": "user", "content": [{"text": user}]}],
            "inferenceConfig": {"maxTokens": 700, "temperature": 0},
        }
    if model_id.startswith("anthropic."):
        return {
            "anthropic_version": "bedrock-2023-05-31",
            "max_tokens": 700,
            "temperature": 0,
            "system": system,
            "messages": [{"role": "user", "content": user}],
        }
    raise ValueError(f"unsupported Bedrock model {model_id}")


def extract_text(model_id, payload):
    if model_id.startswith("amazon.nova"):
        return payload["output"]["message"]["content"][0]["text"]
    return payload["content"][0]["text"]


SYSTEM = (
    "Write a short trip itinerary from the JSON only. "
    "Every rupee amount must appear in the JSON. Do not invent prices, hotels, or trains. "
    "Say that trains are an IRCTC check and are not booked."
)


def narrate_with_bedrock(plan):
    import boto3

    fallback = narrate(plan)
    model_id = settings.BEDROCK_MODEL_ID
    body = build_bedrock_body(model_id, SYSTEM, json.dumps(plan))
    client = boto3.client("bedrock-runtime", region_name=settings.AWS_REGION)
    response = client.invoke_model(modelId=model_id, body=json.dumps(body))
    payload = json.loads(response["body"].read())
    text = extract_text(model_id, payload).strip()
    if not text or not audit(text, plan) or MONEY.search(text) is None:
        return fallback, "template"
    return text, "bedrock"
