from pymongo import MongoClient

uri = 'mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev'
client = MongoClient(uri)
db = client['slr_agentic_db']

for session in db.slr_sessions.find():
    if 'papers' in session:
        for p in session['papers']:
            title = p.get('title', '')
            if 'DHCM' in title:
                print(f"Found in session {session['_id']}: {title}")
                print(f"DOI: {p.get('doi')}")
                print(f"full_text_retrieved: {p.get('full_text_retrieved')}")
