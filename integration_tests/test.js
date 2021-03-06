/* eslint no-undef: "off", no-native-reassign: "off" */

var netloc = '0.0.0.0'
if (typeof window === 'undefined') {
  expect = require('chai').expect
  PouchDB = require('pouchdb-node')
  PouchDB.plugin(require('transform-pouch'))
  pouchSumma = require('pouch-summa')
  fetch = require('node-fetch')
  Promise = require('lie')
  apolloClient = require('apollo-client')
  gql = require('graphql-tag')
  fetch.Promise = Promise
} else {
  expect = chai.expect
  gql = graphqlTag
  process = {env: {}}
  netloc = location.hostname
}

var local
var summa = 'http://' + netloc + ':7896/subdb'
var summa2 = 'http://' + netloc + ':7897/subdb'

function val (v) { return Object({_val: v}) }

describe('integration', function () {
  this.timeout(40000)

  before(function () { // cleaning up local db -- remote doesn't need to be cleared as it should
                 // have been started out clear already.
    return Promise.resolve().then(function () {
      return new PouchDB('pouch-test-db')
    }).then(function (db) {
      local = db
      return local.destroy()
    }).then(function () {
      return new PouchDB('pouch-test-db')
    }).then(function (db) {
      local = db
      local.transform(pouchSumma)
    })
  })

  describe('basic crud', function () {
    it('should add a doc', function () {
      return Promise.resolve().then(function () {
        return fetch(summa + '/docid', {method: 'PUT', body: JSON.stringify({what: 'a doc'})})
      })
    })
  })

  describe('replication to pouchdb', function () {
    it('should replicate from summa root to pouchdb', function () {
      return Promise.resolve().then(function () {
        return PouchDB.replicate(summa, local)
      }).then(function () {
        return local.get('docid')
      }).then(function (doc) {
        expect(doc).to.have.all.keys(['_id', '_rev', 'what'])
        expect(doc.what).to.deep.equal('a doc')
      })
    })

    it('should replicate from pouchdb to summa root', function () {
      var revs = []

      return Promise.resolve().then(function () {
        return local.bulkDocs([
          {_id: 'this', sub: 'this is a document'},
          {_id: 'that', sub: 'that is a document'},
          {_id: 'array', array: [1, 2, 3, 4, 5]},
          {_id: 'complex', array: [
            ['a', {letter: 'a'}],
            {'subarray': [
              1, 2, ['xxx']
            ]
          }, true, 5]}
        ])
      }).then(function (res) {
        revs = res.map(function (r) { return r.rev })
        return PouchDB.replicate(local, summa)
      }).then(function () {
        return Promise.all([
          fetch(summa + '/that').then(function (r) { return r.json() }),
          fetch(summa + '/array').then(function (r) { return r.json() }),
          fetch(summa + '/complex').then(function (r) { return r.json() })
        ]).then(function (vals) {
          var that = vals[0]
          var array = vals[1]
          var complex = vals[2]

          expect(that._id).to.equal('that')
          expect(that._rev).to.equal(revs[1])
          expect(array._id).to.equal('array')
          expect(array._rev).to.equal(revs[2])
          expect(array.array).to.deep.equal({
            '0': val(1),
            '1': val(2),
            '2': val(3),
            '3': val(4),
            '4': val(5)
          })
          expect(complex.array).to.deep.equal({
            '0': {
              '0': val('a'),
              '1': {
                letter: val('a')
              }
            },
            '1': {
              subarray: {
                '0': val(1),
                '1': val(2),
                '2': {
                  '0': val('xxx')
                }
              }
            },
            '2': val(true),
            '3': val(5)
          })
        })
      })
    })

    it('should mess up in both databases then compact pouch', function () {
      return Promise.resolve().then(function () {
        return Promise.all([
          fetch(summa + '/_bulk_docs', {method: 'POST', body: JSON.stringify({
            docs: [
              {_id: 'docid', what: 'a doc', val: 234, _rev: '2-auci39gh2'},
              {_id: 'docid', what: 'a doc', val: 23, _rev: '3-xyxyxy'},
              {_id: 'otherdoc', what: 'something', s: {letter: 'a'}, _rev: '3-xxssyxy'},
              {_id: 'that', _rev: '2-zzzzzz', empty: true}
            ], new_edits: false}
          )}),
          fetch(summa + '/extra/numbers', {method: 'PUT', body: JSON.stringify({
            'one': val(1),
            'two': val(2)
          })})
        ])
      }).then(function () {
        return Promise.all([
          local.bulkDocs([
            {_id: 'that', _rev: '2-zzzzzzwwwww', empty: false, val: 1000},
            {_id: 'docid', what: 'only a doc', _rev: '4-aaaa'},
            {_id: 'docid', what: 'so a doc', _rev: '2-bbbb'},
            {_id: 'docid', what: 'just a doc', _rev: '4-zyz'},
            {_id: 'docid', what: 'maybe a doc', _rev: '3-99999'}
          ], {new_edits: false}),
          local.put({_id: 'otherdoc', what: 'nothing', s: {letter: 'b'}})
        ])
      }).then(function () {
        return local.compact()
      })
    })

    it('should replicate from summa to local', function () {
      return Promise.resolve().then(function () {
        return PouchDB.replicate(summa, local)
      }).then(function () {
        return local.allDocs({
          keys: ['docid', 'otherdoc', 'that', 'extra'],
          include_docs: true
        })
      }).then(function (res) {
        expect(res.rows).to.have.length(4)
        var docs = res.rows.map(function (r) { return r.doc })
        expect(docs[0]._rev).to.equal('4-zyz')
        expect(docs[0].what).to.equal('just a doc')
        expect(docs[1].what).to.equal('something')
        expect(docs[1].s.letter).to.equal('a')
        expect(docs[1]._rev).to.equal('3-xxssyxy')
        expect(docs[2]._rev).to.equal('2-zzzzzzwwwww')
        expect(docs[2].empty).to.equal(false)
        expect(docs[2].val).to.equal(1000)
        expect(docs[3].numbers).to.deep.equal({one: 1, two: 2})
        expect(Object.keys(docs[3])).to.have.length(3)
        expect(docs[3]._rev).to.contain('1-')
      })
    })

    it('should replicate from local to summa', function () {
      debug = true
      return Promise.resolve().then(function () {
        return PouchDB.replicate(local, summa)
      }).then(function () {
        return fetch(summa).then(function (r) { return r.json() })
      }).then(function (superdoc) {
        expect(superdoc.docid).to.deep.equal({what: val('just a doc')})
        expect(superdoc.otherdoc).to.deep.equal({what: val('something'), s: {letter: val('a')}})
        expect(superdoc.that).to.deep.equal({empty: val(false), val: val(1000)})
        expect(superdoc.extra).to.deep.equal({numbers: {one: val(1), two: val(2)}})
      })
    })
  })

  describe('replication between summas', function () {
    it('should replicate the current summa to another', function () {
      return Promise.resolve().then(function () {
        return PouchDB.replicate(summa, summa2)
      }).then(function () {
        return fetch(summa2).then(function (r) { return r.json() })
      }).then(function (superdoc) {
        expect(superdoc.docid).to.deep.equal({what: val('just a doc')})
        expect(superdoc.otherdoc).to.deep.equal({what: val('something'), s: {letter: val('a')}})
        expect(superdoc.that).to.deep.equal({empty: val(false), val: val(1000)})
        expect(superdoc.extra).to.deep.equal({numbers: {one: val(1), two: val(2)}})
      })
    })
  })

  describe('graphql queries with apollo client', function () {
    var ApolloClient = apolloClient.default
    var network = apolloClient.createNetworkInterface(summa + '/_graphql')
    var client = new ApolloClient({networkInterface: network})

    it('should do a basic query', function () {
      return client.query({query: gql`
query {
  docid { what }
  extra {
    numbers { one, two }
  }
}
      `}).then(function (res) {
        expect(res.data).to.deep.equal({
          docid: { what: 'just a doc' },
          extra: {
            numbers: { one: 1, two: 2 }
          }
        })
      })
    })
  })
})
