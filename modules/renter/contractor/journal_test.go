package contractor

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/NebulousLabs/Sia/build"
	"github.com/NebulousLabs/Sia/crypto"
	"github.com/NebulousLabs/Sia/types"
)

func tempFile(t interface {
	Name() string
	Fatal(...interface{})
}) (*os.File, func()) {
	f, err := os.Create(filepath.Join(build.TempDir("contractor", t.Name())))
	if err != nil {
		t.Fatal(err)
	}
	return f, func() {
		f.Close()
		os.RemoveAll(f.Name())
	}
}

func tempJournal(t interface {
	Name() string
	Fatal(...interface{})
}) (*journal, func()) {
	j, err := newJournal(filepath.Join(build.TempDir("contractor", t.Name())), contractorPersist{})
	if err != nil {
		t.Fatal(err)
	}
	return j, func() {
		j.Close()
		os.RemoveAll(j.filename)
	}
}

func TestJournal(t *testing.T) {
	j, cleanup := tempJournal(t)
	defer cleanup()

	us := []journalUpdate{
		updateCachedDownloadRevision{Revision: types.FileContractRevision{}},
	}
	if err := j.update(us); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	var data contractorPersist
	j2, err := openJournal(j.filename, &data)
	if err != nil {
		t.Fatal(err)
	}
	j2.Close()
	if len(data.CachedRevisions) != 1 {
		t.Fatal("openJournal applied updates incorrectly:", data)
	}
}

func TestJournalCheckpoint(t *testing.T) {
	j, cleanup := tempJournal(t)
	defer cleanup()

	var data contractorPersist
	data.BlockHeight = 777
	if err := j.checkpoint(data); err != nil {
		t.Fatal(err)
	}
	if err := j.Close(); err != nil {
		t.Fatal(err)
	}

	data.BlockHeight = 0
	j2, err := openJournal(j.filename, &data)
	if err != nil {
		t.Fatal(err)
	}
	j2.Close()
	if data.BlockHeight != 777 {
		t.Fatal("checkpoint failed:", data)
	}
}

func TestJournalMalformedJSON(t *testing.T) {
	j, cleanup := tempJournal(t)
	defer cleanup()

	// write a valid update
	err := j.update(updateSet{updateCachedDownloadRevision{}})
	if err != nil {
		t.Fatal(err)
	}

	// write a partially-malformed update
	j.f.WriteString(`[{"t":"cachedDownloadRevision","d":{"revision":{"parentid":"1000000000000000000000000000000000000000000000000000000000000000"`)

	// load log
	var data contractorPersist
	j, err = openJournal(j.filename, &data)
	if err != nil {
		t.Fatal(err)
	}
	j.Close()

	// the last update set should have been discarded
	if _, ok := data.CachedRevisions[crypto.Hash{}.String()]; !ok {
		t.Fatal("log was not applied correctly:", data.CachedRevisions)
	}
}

func TestJournalBadChecksum(t *testing.T) {
	// test bad checksum
	j, cleanup := tempJournal(t)
	defer cleanup()

	// write a valid update
	err := j.update(updateSet{updateCachedDownloadRevision{}})
	if err != nil {
		t.Fatal(err)
	}

	// write an update with a bad checksum
	j.f.WriteString(`[{"t":"cachedDownloadRevision","d":{"revision":{"parentid":"2000000000000000000000000000000000000000000000000000000000000000"}},"c":"bad checksum"}]`)

	// load log
	var data contractorPersist
	j, err = openJournal(j.filename, &data)
	if err != nil {
		t.Fatal(err)
	}
	j.Close()

	// the last update set should have been discarded
	if _, ok := data.CachedRevisions[crypto.Hash{}.String()]; !ok {
		t.Fatal("log was not applied correctly:", data)
	}
}

// TestJournalLoadCompat tests that the contractor can convert the previous
// persist file to a journal.
func TestJournalLoadCompat(t *testing.T) {
	// create old persist file
	dir := build.TempDir("contractor", t.Name())
	os.MkdirAll(dir, 0700)
	err := ioutil.WriteFile(filepath.Join(dir, "contractor.json"), []byte(`"Contractor Persistence"
"0.5.2"
{
	"Allowance": {
		"funds": "5000000000000000000000000000",
		"hosts": 42,
		"period": 12960,
		"renewwindow": 6480
	},
	"BlockHeight": 92885,
	"CachedRevisions": [
		{
			"Revision": {
				"parentid": "85a32f6ca706298407668718703b005ab1a558694eab69a2cc33e1bc0d3fb38f",
				"unlockconditions": {
					"publickeys": [
						{"algorithm": "ed25519", "key": "sU1bxlHat5zgjlAqI7UfVPGKFQp3FpcPzGWa6K9ARfk="},
						{"algorithm": "ed25519", "key": "pFrpZJEoH8dF+wQMLwZ6f8N2ghYzXjSCkotoJ0vgAjo="}
					],
					"signaturesrequired": 2
				},
				"newrevisionnumber": 205,
				"newfilesize": 792723456,
				"newfilemerkleroot": "1a6c3bae8f95b168188fcd86461e4b8830a0af5f895b401856e144e6ac833d4d",
				"newwindowstart": 101748,
				"newwindowend": 101892,
				"newvalidproofoutputs": [
					{"value": "912571784802684347584", "unlockhash": "f23541eb5c5de647b56c708e1b4972d0770e802f562c521a5f3e6613bb890999fd6a72c3c4cd" },
					{"value": "54974203903776165095014400", "unlockhash": "e9d49e22328ba38ecc4969cb21f777915ff9828a79438aa408a668434c557d0c119d7d02e798" }
				],
				"newmissedproofoutputs": [
					{"value": "912571784802684347584", "unlockhash": "f23541eb5c5de647b56c708e1b4972d0770e802f562c521a5f3e6613bb890999fd6a72c3c4cd"},
					{"value": "53776524850119119131978612", "unlockhash": "e9d49e22328ba38ecc4969cb21f777915ff9828a79438aa408a668434c557d0c119d7d02e798"},
					{"value": "1197679053657045963035788", "unlockhash": "000000000000000000000000000000000000000000000000000000000000000089eb0d6a8a69"}
				],
				"newunlockhash": "3aa8c31a63d67d0671d924df556a6214057c9fa611fa5607b1bf5d1ec3e861b0cc78fc4e3914"
			},
			"MerkleRoots": [
				"d3c27e3e361f7ff8fbb7aedcb8b24b0613d7e413fc9d7edd8ebbbf9911134487",
				"5bc124f5dcadee196611252eec599096b5146642b6785cac3c0625ce472d863a",
				"ff3bf7ccbc092ce4b851b76587fa3e9decff3f8421d49d6e72069d6cfc46f382"
			]
		}
	],
	"Contracts": [
		{
			"filecontract": {
				"filesize": 0,
				"filemerkleroot": "0000000000000000000000000000000000000000000000000000000000000000",
				"windowstart": 101748,
				"windowend": 101892,
				"payout": "1601052162572897650533988302",
				"validproofoutputs": [
					{"value": "1258611128232554642163168302", "unlockhash": "9a712eceba9f0523522ff9f5687ef6a54e5299d27c632044cd20f207e809fcb306f8373fe12b"},
					{"value": "280000000000000000000000000", "unlockhash": "f879ab09edd4b3650aed02ce6226d4f6a197409d42be84c310a5e86657879a85d9575dc51a0d"}
				],
				"missedproofoutputs": [
					{"value": "1258611128232554642163168302", "unlockhash": "9a712eceba9f0523522ff9f5687ef6a54e5299d27c632044cd20f207e809fcb306f8373fe12b"},
					{"value": "280000000000000000000000000", "unlockhash": "f879ab09edd4b3650aed02ce6226d4f6a197409d42be84c310a5e86657879a85d9575dc51a0d"},
					{"value": "0", "unlockhash": "000000000000000000000000000000000000000000000000000000000000000089eb0d6a8a69"}
				],
				"unlockhash": "4440e7ad4a744a1745797f9180b08aeeb1aa42c3c12729ed063f6ef4897f2cf7599a9efe4059",
				"revisionnumber": 0
			},
			"id": "87893a702b4af71151a853229f7dd4071929b24b4bf1c39bafec551daeaf11de",
			"lastrevision": {
				"parentid": "87893a702b4af71151a853229f7dd4071929b24b4bf1c39bafec551daeaf11de",
				"unlockconditions": {
					"timelock": 0,
					"publickeys": [
						{"algorithm": "ed25519", "key": "ux0dwMoOTt2Q+VlmSy3G59nIn5kwWPrZMUKFphJgIGM="},
						{"algorithm": "ed25519", "key": "5rgAREJJuMrmHfS3vWV0TN2Y8cHZf8UU2CM8BBFX5q4="}
					],
					"signaturesrequired": 2
				},
				"newrevisionnumber": 117,
				"newfilesize": 465567744,
				"newfilemerkleroot": "449af205a10e645324c9016062e843856538122e4044e18b6e93aaab960cd8e6",
				"newwindowstart": 101748,
				"newwindowend": 101892,
				"newvalidproofoutputs": [
					{"value": "1232516258766241909102140813", "unlockhash": "9a712eceba9f0523522ff9f5687ef6a54e5299d27c632044cd20f207e809fcb306f8373fe12b"},
					{"value": "306094869466312733061027489", "unlockhash": "f879ab09edd4b3650aed02ce6226d4f6a197409d42be84c310a5e86657879a85d9575dc51a0d"}
				],
				"newmissedproofoutputs": [
					{"value": "1232516258766241909102140813", "unlockhash": "9a712eceba9f0523522ff9f5687ef6a54e5299d27c632044cd20f207e809fcb306f8373fe12b"},
					{"value": "278593489793967488506695049", "unlockhash": "f879ab09edd4b3650aed02ce6226d4f6a197409d42be84c310a5e86657879a85d9575dc51a0d"},
					{"value": "27501379672345244554332440", "unlockhash": "000000000000000000000000000000000000000000000000000000000000000089eb0d6a8a69"}
				],
				"newunlockhash": "4440e7ad4a744a1745797f9180b08aeeb1aa42c3c12729ed063f6ef4897f2cf7599a9efe4059"
			},
			"lastrevisiontxn": {
				"filecontractrevisions": [
					{
						"parentid": "87893a702b4af71151a853229f7dd4071929b24b4bf1c39bafec551daeaf11de",
						"unlockconditions": {
							"timelock": 0,
							"publickeys": [
								{"algorithm": "ed25519", "key": "ux0dwMoOTt2Q+VlmSy3G59nIn5kwWPrZMUKFphJgIGM="},
								{"algorithm": "ed25519", "key": "5rgAREJJuMrmHfS3vWV0TN2Y8cHZf8UU2CM8BBFX5q4="}
							],
							"signaturesrequired": 2
						},
						"newrevisionnumber": 117,
						"newfilesize": 465567744,
						"newfilemerkleroot": "449af205a10e645324c9016062e843856538122e4044e18b6e93aaab960cd8e6",
						"newwindowstart": 101748,
						"newwindowend": 101892,
						"newvalidproofoutputs": [
							{"value": "1232516258766241909102140813", "unlockhash": "9a712eceba9f0523522ff9f5687ef6a54e5299d27c632044cd20f207e809fcb306f8373fe12b"},
							{"value": "306094869466312733061027489", "unlockhash": "f879ab09edd4b3650aed02ce6226d4f6a197409d42be84c310a5e86657879a85d9575dc51a0d"}
						],
						"newmissedproofoutputs": [
							{"value": "1232516258766241909102140813", "unlockhash": "9a712eceba9f0523522ff9f5687ef6a54e5299d27c632044cd20f207e809fcb306f8373fe12b"},
							{"value": "278593489793967488506695049", "unlockhash": "f879ab09edd4b3650aed02ce6226d4f6a197409d42be84c310a5e86657879a85d9575dc51a0d"},
							{"value": "27501379672345244554332440", "unlockhash": "000000000000000000000000000000000000000000000000000000000000000089eb0d6a8a69"}
						],
						"newunlockhash": "4440e7ad4a744a1745797f9180b08aeeb1aa42c3c12729ed063f6ef4897f2cf7599a9efe4059"
					}
				],
				"transactionsignatures": [
					{
						"parentid": "87893a702b4af71151a853229f7dd4071929b24b4bf1c39bafec551daeaf11de",
						"publickeyindex": 0,
						"coveredfields": {
							"wholetransaction": false,
							"filecontractrevisions": [0]
						},
						"signature": "zs1T+NO5sFR6jVgilYXxJx33gPhd4Y7KRjpsKAG4EFZ7cthgBidXIDkTbOknk8P9Al7bDj1Dq6PMt+Mgvb+tBg=="
					},
					{
						"parentid": "87893a702b4af71151a853229f7dd4071929b24b4bf1c39bafec551daeaf11de",
						"publickeyindex": 1,
						"timelock": 0,
						"coveredfields": {
							"wholetransaction": false,
							"filecontractrevisions": [0]
						},
						"signature": "5jxxxqSaKF/KXNT8oWHiesiHl6l+GHH+zDCSxe3UsQJS+LyB+NY6k+AoQ+7l8ysA5rt/MXt08Gh+iFc95StJCQ=="
					}
				]
			},
			"merkleroots": [
				"5d8c2b8ecb23b0cbbb842f236bca90f0f9a684c0d49e5008fa356a3c75d83764",
				"35e9e31000bdfc6adf1eddbe13d2e584bc274f803f03b23bbf1ac3b3334b7335",
				"9f6b52ff2b68da078648f073e119d030a69137020792bb6ba601590aead4ab76",
				"c327be1fc31360c40f6ed5cd729354f20c820f31970a1093cadd914ab55bfed9",
				"749df474d6ff4c306f8ca8695af352e3596724a286171f495097599b6d6bda61"
			],
			"netaddress": "88.196.244.208:5982",
			"secretkey": [0,0,0,0,0],
			"startheight": 88793,
			"downloadspending": "83886080000000000000000",
			"storagespending": "25189293437743169757705777",
			"uploadspending": "462296186880000000819500",
			"totalcost": "1351052162572897650533988302",
			"contractfee": "30000000000000000000000000",
			"txnfee": "10240000000000000000000000",
			"siafundfee": "62441034340343008370820000"
		}
	],
	"CurrentPeriod": 88788,
	"LastChange": [194,19,235,129,22,141,244,238,202,1,240,253,223,37,173,182,252,119,197,154,77,226,137,98,242,231,164,201,34,102,96,194],
	"OldContracts": null,
	"RenewedIDs": {}
}
`), 0666)
	if err != nil {
		t.Fatal(err)
	}

	// load will fail to load journal, fall back to loading contractor.json,
	// and save data as a new journal
	p := newPersist(dir)
	var data contractorPersist
	err = p.load(&data)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	// second load should find the journal
	var data2 contractorPersist
	p = newPersist(dir)
	err = p.load(&data2)
	if err != nil {
		t.Fatal(err)
	}
	defer p.Close()

	if !reflect.DeepEqual(data, data2) {
		t.Fatal("data mismatch after loading old persist:", data, data2)
	}
}

func BenchmarkUpdateJournal(b *testing.B) {
	j, cleanup := tempJournal(b)
	defer cleanup()

	us := updateSet{
		updateCachedUploadRevision{
			Revision: types.FileContractRevision{
				NewValidProofOutputs:  []types.SiacoinOutput{{}, {}},
				NewMissedProofOutputs: []types.SiacoinOutput{{}, {}},
				UnlockConditions:      types.UnlockConditions{PublicKeys: []types.SiaPublicKey{{}, {}}},
			},
		},
		updateUploadRevision{
			NewRevisionTxn: types.Transaction{
				FileContractRevisions: []types.FileContractRevision{{
					NewValidProofOutputs:  []types.SiacoinOutput{{}, {}},
					NewMissedProofOutputs: []types.SiacoinOutput{{}, {}},
					UnlockConditions:      types.UnlockConditions{PublicKeys: []types.SiaPublicKey{{}, {}}},
				}},
				TransactionSignatures: []types.TransactionSignature{{}, {}},
			},
			NewUploadSpending:  types.SiacoinPrecision,
			NewStorageSpending: types.SiacoinPrecision,
		},
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(us)
	b.SetBytes(int64(buf.Len()))
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if err := j.update(us); err != nil {
			b.Fatal(err)
		}
	}
}
