from pymongo import MongoClient
import codecs
import json
from bson import json_util

uri = 'mongodb+srv://apkflydev:aqv082W6Xy4ercBt@apkflydev.cysnbuw.mongodb.net/?appName=apkflydev'
client = MongoClient(uri)
db = client['slr_agentic_db']

with codecs.open('output3.txt', 'w', 'utf-8') as f:
    # Check all collections for DOI
    for coll_name in db.list_collection_names():
        docs = list(db[coll_name].find({'$or': [
            {'DOI': {'$regex': '37199'}}, 
            {'doi': {'$regex': '37199'}},
            {'Title': {'$regex': 'DHCM'}},
            {'title': {'$regex': 'DHCM'}}
        ]}))
        for doc in docs:
            f.write(f'--- FOUND IN {coll_name} ---\n')
            f.write(json.dumps(doc, indent=2, default=json_util.default) + '\n')
