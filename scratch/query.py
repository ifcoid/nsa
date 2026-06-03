from pymongo import MongoClient
import codecs
uri = 'mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev'
client = MongoClient(uri)
db = client['slr_agentic_db']
with codecs.open('output.txt', 'w', 'utf-8') as f:
    for coll_name in ['slr_screening', 'slr_papers', 'slr_papers_post_dedup']:
        for doc in db[coll_name].find():
            title = doc.get('Title', doc.get('title', ''))
            if 'DHCM' in title or 'Emotion' in title:
                f.write(f'{coll_name} -> {title}\n')
                f.write(f'DOI: {doc.get("DOI", doc.get("doi", ""))}\n')
                f.write(f'session: {doc.get("session_id")}\n\n')
