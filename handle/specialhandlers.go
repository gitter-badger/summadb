package handle

import (
	"bytes"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	db "github.com/fiatjaf/summadb/database"
	responses "github.com/fiatjaf/summadb/handle/responses"
	settings "github.com/fiatjaf/summadb/settings"
)

func DatabaseInfo(w http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)

	lastSeq, err := db.LastSeqAt(ctx.path)
	if err != nil {
		res := responses.UnknownError(err.Error())
		w.WriteHeader(res.Code)
		json.NewEncoder(w).Encode(res)
		return
	}

	res := responses.DatabaseInfo{
		DBName:            ctx.path,
		UpdateSeq:         lastSeq,
		InstanceStartTime: settings.STARTTIME.Unix(),
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(res)
}

func Changes(w http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)

	/* options */
	// always true temporarily: all_docs := flag(r, "style", "all_docs")
	sincep := param(r, "since")
	var since uint64
	if sincep == "now" {
		since = db.GlobalUpdateSeq()
	} else {
		var err error
		since, err = strconv.ParseUint(sincep, 10, 64)
		if err != nil {
			since = 0
		}
	}

	path := db.CleanPath(ctx.path)
	changes, err := db.ListChangesAt(path, since)
	if err != nil {
		res := responses.UnknownError(err.Error())
		w.WriteHeader(res.Code)
		json.NewEncoder(w).Encode(res)
		return
	}

	var lastSeq uint64 = 0
	nchanges := len(changes)
	if nchanges > 0 {
		lastSeq = changes[nchanges-1].Seq
	}

	res := responses.Changes{
		LastSeq: lastSeq,
		Results: changes,
	}
	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(res)
}

/*
   Currently _all_docs does not guarantee key order and should not be used for
   querying or anything. It is here just to provide PouchDB replication support.
*/
func AllDocs(w http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)

	/* options */
	include_docs := flag(r, "include_docs")
	jsonKeys := param(r, "keys")
	// startkey := param(r, "startkey")
	// endkey := param(r, "endkey")
	// descending := flag(r, "descending")
	// skip := param(r, "skip")
	// limit := param(r, "limit")

	path := db.CleanPath(ctx.path)
	tree, err := db.GetTreeAt(path)
	if err != nil {
		res := responses.UnknownError(err.Error())
		w.WriteHeader(res.Code)
		json.NewEncoder(w).Encode(res)
		return
	}

	res := responses.AllDocs{
		TotalRows: 0,
		Offset:    0, // temporary
		Rows:      make([]responses.Row, 0),
	}

	if jsonKeys != "" {
		// in case "keys" was provided we can just pick the requested keys from our tree
		var keys []string
		err = json.Unmarshal([]byte(jsonKeys), &keys)
		if err != nil {
			res := responses.BadRequest()
			w.WriteHeader(res.Code)
			json.NewEncoder(w).Encode(res)
			return
		}

		for _, id := range keys {
			var doc interface{}
			var ok bool
			if doc, ok = tree[id]; !ok || strings.HasPrefix(id, "_") {
				res.Rows = append(res.Rows, responses.Row{
					Key:   id,
					Error: (responses.NotFound()).Reason,
				})
			}
			rev := string(db.GetRev(path + "/" + db.EscapeKey(id)))
			row := responses.Row{
				Id:    id,
				Key:   id,
				Value: map[string]interface{}{"rev": rev},
			}
			if include_docs {
				row.Doc = doc.(map[string]interface{})
				row.Doc["_id"] = id
				row.Doc["_rev"] = rev
				delete(row.Doc, "_val") // pouchdb doesn't like top-level keys starting with _
			}
			res.Rows = append(res.Rows, row)
			res.TotalRows += 1
		}
	} else {
		// otherwise iterate through the entire tree
		for id, doc := range tree {
			if strings.HasPrefix(id, "_") {
				continue
			}

			rev := string(db.GetRev(path + "/" + db.EscapeKey(id)))
			row := responses.Row{
				Id:    id,
				Key:   id,
				Value: map[string]interface{}{"rev": rev},
			}
			if include_docs {
				row.Doc = doc.(map[string]interface{})
				row.Doc["_id"] = id
				row.Doc["_rev"] = rev
				delete(row.Doc, "_val") // pouchdb doesn't like top-level keys starting with _
			}
			res.Rows = append(res.Rows, row)
			res.TotalRows += 1
		}
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(res)
}

// this method only exists for compatibility with PouchDB and should not be used elsewhere.
func BulkGet(w http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)
	var ireqs interface{}

	/* options */
	revs := flag(r, "revs")

	var ok bool
	if ireqs, ok = ctx.jsonBody["docs"]; !ok {
		res := responses.BadRequest("You were supposed to request some docs specified by their ids, and you didn't.")
		w.WriteHeader(res.Code)
		json.NewEncoder(w).Encode(res)
		return
	}

	reqs := ireqs.([]interface{})
	res := responses.BulkGet{
		Results: make([]responses.BulkGetResult, len(reqs)),
	}
	for i, ireq := range reqs {
		req := ireq.(map[string]interface{})
		res.Results[i] = responses.BulkGetResult{
			Docs: make([]responses.DocOrError, 1),
		}

		iid, ok := req["id"]
		if !ok {
			err := responses.BadRequest("missing id")
			res.Results[i].Docs[0].Error = &err
			continue
		}
		id := iid.(string)
		res.Results[i].Id = id

		path := db.CleanPath(ctx.path) + "/" + db.EscapeKey(id)
		rev := string(db.GetRev(path))
		doc, err := db.GetTreeAt(path)
		if err != nil {
			err := responses.NotFound()
			res.Results[i].Docs[0].Error = &err
			continue
		}

		doc["_id"] = id
		doc["_rev"] = rev
		delete(doc, "_val") // pouchdb doesn't like top-level keys starting with _

		if revs {
			// magic method to fetch _revisions
			// docs["_revisions"] = ...
		}

		res.Results[i].Docs[0].Ok = &doc
	}

	w.Header().Add("Content-Type", "application/json")
	w.WriteHeader(200)
	json.NewEncoder(w).Encode(res)
}

// this method only exists for compatibility with PouchDB and should not be used elsewhere.
func BulkDocs(w http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)
	var idocs interface{}
	var iid interface{}
	var irev interface{}
	var id string
	var rev string
	var ok bool

	if idocs, ok = ctx.jsonBody["docs"]; !ok {
		res := responses.BadRequest("You're supposed to send an array of docs to input on the database, and you didn't.")
		w.WriteHeader(res.Code)
		json.NewEncoder(w).Encode(res)
		return
	}

	/* options */
	new_edits := true
	if inew_edits, ok := ctx.jsonBody["new_edits"]; ok {
		new_edits = inew_edits.(bool)
	}

	path := db.CleanPath(ctx.path)
	docs := idocs.([]interface{})
	res := make([]responses.BulkDocsResult, len(docs))

	var err error
	if new_edits { /* procedure for normal document saving */
		for i, idoc := range docs {

			var newrev string
			localDoc := false
			doc := idoc.(map[string]interface{})
			if iid, ok = doc["_id"]; ok {
				id = iid.(string)
				if strings.HasPrefix(id, "_local/") {
					localDoc = true
				}
				if irev, ok = doc["_rev"]; ok {
					rev = irev.(string)
				} else {
					rev = ""
				}
			} else {
				id = db.Random(5)
			}

			if !localDoc {
				// procedure for normal documents

				// check rev matching:
				if iid != nil /* when iid is nil that means the doc had no _id, so we don't have to check. */ {
					currentRev := db.GetRev(path + "/" + db.EscapeKey(id))
					if string(currentRev) != rev && !bytes.HasPrefix(currentRev, []byte{'0', '-'}) {
						// rev is not matching, don't input this document
						e := responses.ConflictError()
						res[i] = responses.BulkDocsResult{
							Id:     id,
							Error:  e.Error,
							Reason: e.Reason,
						}
						continue
					}
				}

				// proceed to write the document.
				newrev, err = db.ReplaceTreeAt(path+"/"+db.EscapeKey(id), doc, false)
				if err != nil {
					e := responses.UnknownError()
					res[i] = responses.BulkDocsResult{
						Id:     id,
						Error:  e.Error,
						Reason: e.Reason,
					}
					continue
				}

			} else {
				// procedure for local documents

				// check rev matching:
				currentRev := db.GetLocalDocRev(path + "/" + id)
				if currentRev != "0-0" && currentRev != rev {
					// rev is not matching, don't input this document
					e := responses.ConflictError()
					res[i] = responses.BulkDocsResult{
						Id:     id,
						Error:  e.Error,
						Reason: e.Reason,
					}
					continue
				}

				// proceed to write the document.
				newrev, err = db.SaveLocalDocAt(path+"/"+id, doc)
				if err != nil {
					e := responses.UnknownError()
					res[i] = responses.BulkDocsResult{
						Id:     id,
						Error:  e.Error,
						Reason: e.Reason,
					}
					continue
				}
			}

			res[i] = responses.BulkDocsResult{
				Id:  id,
				Ok:  true,
				Rev: newrev,
			}
		}
	} else { /* procedure for replication-type _bulk_docs (new_edits=false)*/
		for i, idoc := range docs {
			id = ""
			rev = ""
			doc := idoc.(map[string]interface{})
			if iid, ok = doc["_id"]; ok {
				id = iid.(string)
				if irev, ok = doc["_rev"]; ok {
					rev = irev.(string)
				}
			}
			if id == "" || rev == "" {
				e := responses.UnknownError()
				res[i] = responses.BulkDocsResult{
					Id:     id,
					Error:  e.Error,
					Reason: e.Reason,
				}
				continue
			}

			// proceed to write
			currentRev := db.GetRev(path + "/" + db.EscapeKey(id))
			if rev < string(currentRev) {
				// will write only the rev if it is not the winning rev
				db.AcknowledgeRevFor(path+"/"+db.EscapeKey(id), rev)

			} else {
				// otherwise will write the whole doc normally (replacing the current)
				rev, err = db.ReplaceTreeAt(path+"/"+db.EscapeKey(id), doc, true)
			}

			// handle write error
			if err != nil {
				e := responses.UnknownError()
				res[i] = responses.BulkDocsResult{
					Id:     id,
					Error:  e.Error,
					Reason: e.Reason,
				}
				continue
			}

			res[i] = responses.BulkDocsResult{
				Id:  id,
				Ok:  true,
				Rev: rev,
			}
		}
	}

	w.WriteHeader(201)
	json.NewEncoder(w).Encode(res)
}

// this method only exists for compatibility with PouchDB and should not be used elsewhere.
func RevsDiff(w http.ResponseWriter, r *http.Request) {
	ctx := getContext(r)

	path := db.CleanPath(ctx.path)

	res := make(map[string]responses.RevsDiffResult)
	for id, irevs := range ctx.jsonBody {
		missing := make([]string, 0)

		currentRevb, err := db.GetValueAt(path + "/" + id + "/_rev")
		if err != nil {
			/* no _rev for this id, means it has never been inserted in this database.
			   let's say we miss all the revs. */
			for _, irev := range irevs.([]interface{}) {
				rev := irev.(string)
				missing = append(missing, rev)
			}
		} else {
			/* otherwise we will say we have the current rev and none of the others,
			   because that's the truth. */
			currentRev := string(currentRevb)
			for _, irev := range irevs.([]interface{}) {
				rev := irev.(string)
				if rev != currentRev {
					missing = append(missing, rev)
				}
			}
		}

		res[id] = responses.RevsDiffResult{Missing: missing}
	}

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(res)
}
