import os
import requests
from pymongo import MongoClient

# 1. Connect to Mongo
mongo_uri = "mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev"
client = MongoClient(mongo_uri)
db = client["slr_agentic_db"]
coll = db["screening_papers"]

# Get all DOIs from Mongo
mongo_dois = {}
for p in coll.find({"status": "INCLUDE"}):
    doi = p.get("doi") or p.get("DOI") or ""
    if doi:
        doi = doi.replace("https://doi.org/", "").replace("http://doi.org/", "").strip().lower()
        mongo_dois[doi] = True

# 2. Connect to Qdrant
qdrant_url = "https://67983937-4c0e-403a-9f44-9934c86743e9.australia-southeast1-0.gcp.cloud.qdrant.io"
qdrant_key = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3MiOiJtIiwic3ViamVjdCI6ImFwaS1rZXk6YTAwZTdlYjItNTM3Yy00MjM5LWIwM2ItZDk1OTE0NGU3ZWEyIn0.Jf0_GsAldUY4-SICiEcYfBG36l5vE2DQmKdsJOjoN94"

res = requests.post(
    f"{qdrant_url}/collections/scientific_articles/points/scroll",
    headers={"Content-Type": "application/json", "api-key": qdrant_key},
    json={"limit": 1000, "with_payload": ["doi", "title"], "with_vector": False}
)

qdrant_dois = {}
points = res.json().get("result", {}).get("points", [])
for pt in points:
    payload = pt.get("payload", {})
    d = payload.get("doi", "")
    if d:
        d = d.replace("https://doi.org/", "").replace("http://doi.org/", "").strip().lower()
        title = payload.get("title", "Unknown")
        qdrant_dois[d] = title

# 3. Compare
print("Missing from Mongo:")
for qd, qt in qdrant_dois.items():
    if qd not in mongo_dois:
        print(f"- DOI: {qd} | Title: {qt}")
