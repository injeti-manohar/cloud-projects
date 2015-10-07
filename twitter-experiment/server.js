var r = require('rethinkdb');
var express = require('express');

var bodyParser = require('body-parser');
var app = express();

var http = require('http').Server(app);
var io = require('socket.io')(http);

// read in the config
var config = require(__dirname + '/config.js');

// setup static content
app.use(express.static(__dirname + "/public"));

// parse application/json
app.use(bodyParser.json());

// middleware that gets the connection to the db
// and makes it available to the request object
function createConnection(req, res, next) {
    r.connect(config.rethinkdb).then(function(conn) {
        req._rdbConn = conn;
        next();
    }).error(handleError(res));
}

// middleware that closes the connection
function closeConnection(req, res, next) {
    req._rdbConn.close();
}

// handle 500 gracefully
function handleError(res) {
    return function(error) {
        res.send(500, {error: error.message});
    }
}

app.use(createConnection);

app.route('/tweets').get(getTweets);

app.use(closeConnection);

// routes
function getTweets(req, res, next) {
    r.table(config.rethinkdb.table).pluck(['id_str', 'geo', 'text'])
        .run(req._rdbConn).then(function(cursor) {
            return cursor.toArray();
        }).then(function(result) {
            res.send(JSON.stringify(result));
        }).error(handleError(res))
        .finally(next);
}

function startListening() {
    r.connect({host: config.rethinkdb.host, port: config.rethinkdb.port}, function(err, conn) {
        if (err) throw err;
        var connection = conn;
        r.db(config.rethinkdb.db).table(config.rethinkdb.table).changes()
            .run(connection, function(err, cursor) {
                if (err) throw err;
                cursor.each(function(err, row) {
                    if (err) throw err;
                    console.log(JSON.stringify(row, null, 2));
                });
            });
    });
}

io.on('connection', function(socket) {
    console.log("a user connected");
});

// start listening
http.listen(config.express.port, function() {
    console.log('listening on 3000');
    startListening();
});
