import os
import requests
from pymongo import MongoClient

# 1. Connect to Mongo
mongo_uri = "mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev"
client = MongoClient(mongo_uri)
db = client["slr_agentic_db"]
coll = db["slr_screening"]

# Get DOIs from Mongo (session filter NOT strictly needed, but let's just get ALL DOIs from Mongo first)
mongo_dois = {}
for p in coll.find({"$or": [{"Final_Decision": "INCLUDE"}, {"Final_Decision": "", "Screener_1_Decision": "INCLUDE"}]}):
    doi = p.get("doi") or p.get("DOI") or ""
    if doi:
        doi = doi.replace("https://doi.org/", "").replace("http://doi.org/", "").strip().lower()
        mongo_dois[doi] = p.get("title", "Unknown")

print(f"Total Mongo INCLUDE DOIs: {len(mongo_dois)}")

# 2. Connect to Qdrant
qdrant_url = "https://67983937-4c0e-403a-9f44-9934c86743e9.australia-southeast1-0.gcp.cloud.qdrant.io"
qdrant_key = "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJhY2Nlc3MiOiJtIiwic3ViamVjdCI6ImFwaS1rZXk6YTAwZTdlYjItNTM3Yy00MjM5LWIwM2ItZDk1OTE0NGU3ZWEyIn0.Jf0_GsAldUY4-SICiEcYfBG36l5vE2DQmKdsJOjoN94"

qdrant_dois = {}
offset = None

while True:
    payload = {"limit": 10000, "with_payload": ["doi", "title"], "with_vector": False}
    if offset:
        payload["offset"] = offset

    res = requests.post(
        f"{qdrant_url}/collections/scientific_articles/points/scroll",
        headers={"Content-Type": "application/json", "api-key": qdrant_key},
        json=payload
    )
    
    if res.status_code != 200:
        print(f"Qdrant Error: {res.text}")
        break

    data = res.json().get("result", {})
    points = data.get("points", [])
    
    for pt in points:
        pld = pt.get("payload", {})
        d = pld.get("doi", "")
        if d:
            d = d.replace("https://doi.org/", "").replace("http://doi.org/", "").strip().lower()
            title = pld.get("title", "Unknown")
            qdrant_dois[d] = title
            
    offset = data.get("next_page_offset")
    if not offset:
        break

print(f"Total Qdrant DOIs: {len(qdrant_dois)}")

# 3. Compare
print("\nDOIs in Qdrant but NOT in Mongo INCLUDE:")
missing = []
for qd, qt in qdrant_dois.items():
    if qd not in mongo_dois:
        missing.append(f"- {qt} (DOI: {qd})")

print(f"Found {len(missing)} missing.")
for m in missing[:10]: # Print top 10
    print(m.encode("ascii", "ignore").decode("ascii"))
