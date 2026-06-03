from pymongo import MongoClient
import codecs

uri = 'mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev'
client = MongoClient(uri)
db = client['slr_agentic_db']
with codecs.open('debug.txt', 'w', 'utf-8') as f:
    docs = list(db.slr_screening.find({'session_id': 'disertasi'}))
    for doc in docs:
        title = doc.get('Title', doc.get('title', ''))
        if 'DHCM' in title:
            f.write(f'Title: {title}\n')
            f.write(f'Full_Text_Retrieved (cap): {doc.get("Full_Text_Retrieved")}\n')
            f.write(f'full_text_retrieved (low): {doc.get("full_text_retrieved")}\n')
            f.write(f'DOI: {doc.get("DOI")}\n')
            f.write('-'*20 + '\n')
