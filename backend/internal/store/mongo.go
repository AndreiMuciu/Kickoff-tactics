package store

import (
	"context"
	"errors"
	"regexp"
	"strings"
	"time"

	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"

	"footy/internal/game"
)

// Mongo works against a local mongod or MongoDB Atlas (mongodb+srv:// URI).
// Every filter is built from server-side values of fixed type (plain strings,
// never client-supplied documents), so no input can inject query operators.
type Mongo struct {
	client   *mongo.Client
	users    *mongo.Collection
	sessions *mongo.Collection
	friends  *mongo.Collection
	messages *mongo.Collection
	runs     *mongo.Collection
	series   *mongo.Collection
	lobbies  *mongo.Collection
	meta     *mongo.Collection
	tokens   *mongo.Collection
	games    *mongo.Collection
	feedback *mongo.Collection
}

type sessionDoc struct {
	ID        string    `bson:"_id"`
	UserID    string    `bson:"userid"`
	CreatedAt time.Time `bson:"createdAt"`
}

func id(v string) bson.D { return bson.D{{Key: "_id", Value: v}} }

func NewMongo(ctx context.Context, uri, db string) (*Mongo, error) {
	opts := options.Client().ApplyURI(uri).
		SetServerSelectionTimeout(5 * time.Second).
		SetConnectTimeout(5 * time.Second).
		SetAppName("footy").
		// Keep a few connections open so a quiet server answers without a
		// fresh TLS handshake to Atlas; compress traffic where the server allows.
		SetMinPoolSize(4).
		SetCompressors([]string{"zstd", "snappy"})
	client, err := mongo.Connect(opts)
	if err != nil {
		return nil, err
	}
	if err := client.Ping(ctx, nil); err != nil {
		_ = client.Disconnect(ctx)
		return nil, err
	}
	d := client.Database(db)
	m := &Mongo{client: client, users: d.Collection("users"), sessions: d.Collection("sessions"),
		friends: d.Collection("friendships"), messages: d.Collection("messages"), runs: d.Collection("runs"), series: d.Collection("series"), lobbies: d.Collection("lobbies"), meta: d.Collection("meta"), tokens: d.Collection("tokens"),
		games: d.Collection("games"), feedback: d.Collection("feedback")}
	indexes := []struct {
		c *mongo.Collection
		m mongo.IndexModel
	}{
		{m.users, mongo.IndexModel{Keys: bson.D{{Key: "namekey", Value: 1}}, Options: options.Index().SetUnique(true)}},
		// Sessions and old chat lines expire on their own.
		{m.sessions, mongo.IndexModel{Keys: bson.D{{Key: "createdAt", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(int32(SessionTTL.Seconds()))}},
		{m.messages, mongo.IndexModel{Keys: bson.D{{Key: "at", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(int32(MessageTTL.Seconds()))}},
		{m.messages, mongo.IndexModel{Keys: bson.D{{Key: "pair", Value: 1}, {Key: "at", Value: -1}}}},
		{m.friends, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}}}},
		{m.lobbies, mongo.IndexModel{Keys: bson.D{{Key: "users", Value: 1}, {Key: "status", Value: 1}}}},
		{m.lobbies, mongo.IndexModel{Keys: bson.D{{Key: "createdat", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(7 * 24 * 3600)}},
		{m.users, mongo.IndexModel{Keys: bson.D{{Key: "lp", Value: -1}}}},
		// Sparse: accounts without an address are not in the index at all.
		{m.users, mongo.IndexModel{Keys: bson.D{{Key: "emailkey", Value: 1}}, Options: options.Index().SetUnique(true).SetSparse(true)}},
		{m.tokens, mongo.IndexModel{Keys: bson.D{{Key: "expires", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(0)}},
		{m.tokens, mongo.IndexModel{Keys: bson.D{{Key: "userid", Value: 1}}}},
		{m.sessions, mongo.IndexModel{Keys: bson.D{{Key: "userid", Value: 1}}}},
		{m.series, mongo.IndexModel{Keys: bson.D{{Key: "a", Value: 1}, {Key: "status", Value: 1}}}},
		{m.series, mongo.IndexModel{Keys: bson.D{{Key: "b", Value: 1}, {Key: "status", Value: 1}}}},
		{m.series, mongo.IndexModel{Keys: bson.D{{Key: "createdat", Value: 1}}, Options: options.Index().SetExpireAfterSeconds(24 * 3600)}},
		{m.friends, mongo.IndexModel{Keys: bson.D{{Key: "b", Value: 1}}}},
		{m.games, mongo.IndexModel{Keys: bson.D{{Key: "userid", Value: 1}, {Key: "at", Value: -1}}}},
		{m.feedback, mongo.IndexModel{Keys: bson.D{{Key: "at", Value: -1}}}},
		{m.users, mongo.IndexModel{Keys: bson.D{{Key: "deletedat", Value: 1}}, Options: options.Index().SetSparse(true)}},
		{m.games, mongo.IndexModel{Keys: bson.D{{Key: "day", Value: 1}, {Key: "points", Value: -1}, {Key: "gd", Value: -1}, {Key: "at", Value: 1}}, Options: options.Index().SetSparse(true)}},
	}
	for _, ix := range indexes {
		if _, err := ix.c.Indexes().CreateOne(ctx, ix.m); err != nil {
			_ = client.Disconnect(ctx)
			return nil, err
		}
	}
	return m, nil
}

func (m *Mongo) Name() string { return "mongodb" }

func (m *Mongo) Close(ctx context.Context) error { return m.client.Disconnect(ctx) }

func notFound(err error) error {
	if errors.Is(err, mongo.ErrNoDocuments) {
		return ErrNotFound
	}
	return err
}

func (m *Mongo) CreateUser(ctx context.Context, u *User) error {
	if _, err := m.users.InsertOne(ctx, u); err != nil {
		return dupUser(err)
	}
	return nil
}

// dupUser tells a taken username from a taken email address.
func dupUser(err error) error {
	if mongo.IsDuplicateKeyError(err) {
		if strings.Contains(err.Error(), "emailkey") {
			return ErrEmailTaken
		}
		return ErrConflict
	}
	return err
}

func (m *Mongo) UserByEmail(ctx context.Context, key string) (*User, error) {
	if key == "" {
		return nil, ErrNotFound
	}
	var u User
	if err := m.users.FindOne(ctx, bson.D{{Key: "emailkey", Value: key}}).Decode(&u); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

func (m *Mongo) SetEmail(ctx context.Context, uid, email, key string) error {
	res, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: bson.D{
		{Key: "email", Value: email}, {Key: "emailkey", Value: key}, {Key: "verified", Value: false}}}})
	if err != nil {
		return dupUser(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) SetVerified(ctx context.Context, uid, key string) error {
	if key == "" {
		return ErrNotFound
	}
	res, err := m.users.UpdateOne(ctx, bson.D{{Key: "_id", Value: uid}, {Key: "emailkey", Value: key}},
		bson.D{{Key: "$set", Value: bson.D{{Key: "verified", Value: true}}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) SetPassword(ctx context.Context, uid, hash string) error {
	res, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: bson.D{{Key: "passhash", Value: hash}}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) SetPlan(ctx context.Context, uid string, plan []string) error {
	res, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: bson.D{{Key: "plan", Value: plan}}}})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) SetAchieved(ctx context.Context, uid string, got map[string]time.Time) error {
	set := bson.D{}
	for k, v := range got {
		set = append(set, bson.E{Key: "achieved." + k, Value: v})
	}
	if len(set) == 0 {
		return nil
	}
	// Accounts without any achievement may hold `achieved: null`; a field
	// cannot be set inside null, so make it a document first.
	if _, err := m.users.UpdateOne(ctx, bson.D{{Key: "_id", Value: uid}, {Key: "achieved", Value: nil}}, bson.D{{Key: "$set", Value: bson.D{{Key: "achieved", Value: bson.D{}}}}}); err != nil {
		return err
	}
	_, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: set}})
	return err
}

func (m *Mongo) SetAchSeen(ctx context.Context, uid string, at time.Time) error {
	_, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: bson.D{{Key: "achseen", Value: at}}}})
	return err
}

func (m *Mongo) AddGame(ctx context.Context, g *GameRecord) error {
	_, err := m.games.InsertOne(ctx, g)
	return err
}

func (m *Mongo) ListGames(ctx context.Context, userID string, skip, limit int) ([]GameRecord, int, error) {
	filter := bson.D{{Key: "userid", Value: userID}}
	total, err := m.games.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, err
	}
	cur, err := m.games.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "at", Value: -1}, {Key: "_id", Value: -1}}).SetSkip(int64(skip)).SetLimit(int64(limit)))
	if err != nil {
		return nil, 0, err
	}
	out := []GameRecord{}
	if err := cur.All(ctx, &out); err != nil {
		return nil, 0, err
	}
	return out, int(total), nil
}

func (m *Mongo) SetFeedbackAsked(ctx context.Context, uid string, at time.Time) error {
	_, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: bson.D{{Key: "feedbackasked", Value: at}}}})
	return err
}

func (m *Mongo) AddFeedback(ctx context.Context, f *Feedback) error {
	_, err := m.feedback.InsertOne(ctx, f)
	return err
}

func (m *Mongo) AddStats(ctx context.Context, uid string, delta map[string]int) error {
	inc := bson.D{}
	for k, v := range delta {
		if v != 0 {
			inc = append(inc, bson.E{Key: "stats." + k, Value: v})
		}
	}
	if len(inc) == 0 {
		return nil
	}
	// Accounts used to be created with `stats: null`, and $inc cannot add a
	// field inside null: turn it into an empty document first.
	if _, err := m.users.UpdateOne(ctx, bson.D{{Key: "_id", Value: uid}, {Key: "stats", Value: nil}}, bson.D{{Key: "$set", Value: bson.D{{Key: "stats", Value: bson.D{}}}}}); err != nil {
		return err
	}
	_, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$inc", Value: inc}})
	return err
}

func (m *Mongo) PutToken(ctx context.Context, t *Token) error {
	_, err := m.tokens.InsertOne(ctx, t)
	return err
}

func (m *Mongo) TakeToken(ctx context.Context, hash string) (*Token, error) {
	var t Token
	if err := m.tokens.FindOneAndDelete(ctx, id(hash)).Decode(&t); err != nil {
		return nil, notFound(err)
	}
	// The TTL monitor runs about once a minute; enforce expiry here too.
	if time.Now().After(t.Expires) {
		return nil, ErrNotFound
	}
	return &t, nil
}

func (m *Mongo) DeleteTokens(ctx context.Context, userID, kind string) error {
	_, err := m.tokens.DeleteMany(ctx, bson.D{{Key: "userid", Value: userID}, {Key: "kind", Value: kind}})
	return err
}

func (m *Mongo) DeleteUserSessions(ctx context.Context, userID, keep string) error {
	_, err := m.sessions.DeleteMany(ctx, bson.D{{Key: "userid", Value: userID}, {Key: "_id", Value: bson.D{{Key: "$ne", Value: keep}}}})
	return err
}

func (m *Mongo) UserByName(ctx context.Context, key string) (*User, error) {
	var u User
	if err := m.users.FindOne(ctx, bson.D{{Key: "namekey", Value: key}}).Decode(&u); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

func (m *Mongo) UserByID(ctx context.Context, uid string) (*User, error) {
	var u User
	if err := m.users.FindOne(ctx, id(uid)).Decode(&u); err != nil {
		return nil, notFound(err)
	}
	return &u, nil
}

func (m *Mongo) UsersByIDs(ctx context.Context, ids []string) (map[string]*User, error) {
	out := map[string]*User{}
	if len(ids) == 0 {
		return out, nil
	}
	cur, err := m.users.Find(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$in", Value: ids}}}})
	if err != nil {
		return nil, err
	}
	var list []User
	if err := cur.All(ctx, &list); err != nil {
		return nil, err
	}
	for i := range list {
		out[list[i].ID] = &list[i]
	}
	return out, nil
}

func (m *Mongo) TouchUser(ctx context.Context, uid string, at time.Time) error {
	_, err := m.users.UpdateOne(ctx, id(uid), bson.D{{Key: "$set", Value: bson.D{{Key: "lastseen", Value: at}}}})
	return err
}

func (m *Mongo) CreateSession(ctx context.Context, h, userID string) error {
	_, err := m.sessions.InsertOne(ctx, sessionDoc{ID: h, UserID: userID, CreatedAt: time.Now().UTC()})
	return err
}

func (m *Mongo) SessionUser(ctx context.Context, h string) (string, error) {
	var s sessionDoc
	if err := m.sessions.FindOne(ctx, id(h)).Decode(&s); err != nil {
		return "", notFound(err)
	}
	// The TTL monitor only runs about once a minute; enforce expiry here too.
	if s.UserID == "" || time.Since(s.CreatedAt) > SessionTTL {
		return "", ErrNotFound
	}
	return s.UserID, nil
}

func (m *Mongo) DeleteSession(ctx context.Context, h string) error {
	_, err := m.sessions.DeleteOne(ctx, id(h))
	return err
}

func (m *Mongo) GetFriendship(ctx context.Context, fid string) (*Friendship, error) {
	var f Friendship
	if err := m.friends.FindOne(ctx, id(fid)).Decode(&f); err != nil {
		return nil, notFound(err)
	}
	return &f, nil
}

func (m *Mongo) PutFriendship(ctx context.Context, f *Friendship) error {
	_, err := m.friends.ReplaceOne(ctx, id(f.ID), f, options.Replace().SetUpsert(true))
	return err
}

func (m *Mongo) DeleteFriendship(ctx context.Context, fid string) error {
	_, err := m.friends.DeleteOne(ctx, id(fid))
	return err
}

func (m *Mongo) ListFriendships(ctx context.Context, userID string) ([]Friendship, error) {
	filter := bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: userID}}, bson.D{{Key: "b", Value: userID}}}}}
	cur, err := m.friends.Find(ctx, filter, options.Find().SetSort(bson.D{{Key: "createdat", Value: 1}}).SetLimit(500))
	if err != nil {
		return nil, err
	}
	var out []Friendship
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (m *Mongo) AddMessage(ctx context.Context, msg *Message) error {
	_, err := m.messages.InsertOne(ctx, msg)
	return err
}

func (m *Mongo) ListMessages(ctx context.Context, pair string, limit int) ([]Message, error) {
	cur, err := m.messages.Find(ctx, bson.D{{Key: "pair", Value: pair}},
		options.Find().SetSort(bson.D{{Key: "at", Value: -1}}).SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	var out []Message
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

func (m *Mongo) GetRun(ctx context.Context, owner string) (*game.Run, error) {
	var r game.Run
	if err := m.runs.FindOne(ctx, id(owner)).Decode(&r); err != nil {
		return nil, notFound(err)
	}
	return &r, nil
}

func (m *Mongo) SaveRun(ctx context.Context, run *game.Run) error {
	next := *run
	next.Version++
	if run.Version == 0 {
		if _, err := m.runs.InsertOne(ctx, &next); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return ErrConflict
			}
			return err
		}
	} else {
		filter := bson.D{{Key: "_id", Value: run.SessionID}, {Key: "version", Value: run.Version}}
		res, err := m.runs.ReplaceOne(ctx, filter, &next)
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return ErrConflict
		}
	}
	run.Version = next.Version
	return nil
}

func (m *Mongo) GetSeries(ctx context.Context, lid string) (*Series, error) {
	var l Series
	if err := m.series.FindOne(ctx, id(lid)).Decode(&l); err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

func (m *Mongo) ActiveSeries(ctx context.Context, userID string) (*Series, error) {
	filter := bson.D{
		{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: userID}}, bson.D{{Key: "b", Value: userID}}}},
		{Key: "status", Value: bson.D{{Key: "$in", Value: bson.A{"invited", "live"}}}},
	}
	var l Series
	err := m.series.FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "createdat", Value: -1}})).Decode(&l)
	if err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

func (m *Mongo) SaveSeries(ctx context.Context, l *Series) error {
	next := *l
	next.Version++
	if l.Version == 0 {
		if _, err := m.series.InsertOne(ctx, &next); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return ErrConflict
			}
			return err
		}
	} else {
		res, err := m.series.ReplaceOne(ctx, bson.D{{Key: "_id", Value: l.ID}, {Key: "version", Value: l.Version}}, &next)
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return ErrConflict
		}
	}
	l.Version = next.Version
	return nil
}

func (m *Mongo) DeleteSeries(ctx context.Context, lid string) error {
	_, err := m.series.DeleteOne(ctx, id(lid))
	return err
}

func (m *Mongo) GetLobby(ctx context.Context, lid string) (*Lobby, error) {
	var l Lobby
	if err := m.lobbies.FindOne(ctx, id(lid)).Decode(&l); err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

func (m *Mongo) ActiveLobby(ctx context.Context, userID string) (*Lobby, error) {
	var l Lobby
	filter := bson.D{
		{Key: "status", Value: bson.D{{Key: "$in", Value: bson.A{"open", "live"}}}},
		{Key: "$or", Value: bson.A{bson.D{{Key: "users", Value: userID}}, bson.D{{Key: "invited", Value: userID}}}},
	}
	if err := m.lobbies.FindOne(ctx, filter, options.FindOne().SetSort(bson.D{{Key: "createdat", Value: -1}})).Decode(&l); err != nil {
		return nil, notFound(err)
	}
	return &l, nil
}

func (m *Mongo) SaveLobby(ctx context.Context, l *Lobby) error {
	next := *l
	next.Version++
	if l.Version == 0 {
		if _, err := m.lobbies.InsertOne(ctx, &next); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return ErrConflict
			}
			return err
		}
	} else {
		res, err := m.lobbies.ReplaceOne(ctx, bson.D{{Key: "_id", Value: l.ID}, {Key: "version", Value: l.Version}}, &next)
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return ErrConflict
		}
	}
	l.Version = next.Version
	return nil
}

func (m *Mongo) GetQueue(ctx context.Context, mode string) (*Queue, error) {
	var q Queue
	err := m.meta.FindOne(ctx, id("queue:"+mode)).Decode(&q)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return &Queue{ID: mode}, nil
	}
	if err != nil {
		return nil, err
	}
	q.ID = mode
	return &q, nil
}

func (m *Mongo) SaveQueue(ctx context.Context, q *Queue) error {
	key := "queue:" + q.ID
	next := *q
	next.ID = key
	next.Version++
	if q.Version == 0 {
		if _, err := m.meta.InsertOne(ctx, &next); err != nil {
			if mongo.IsDuplicateKeyError(err) {
				return ErrConflict
			}
			return err
		}
	} else {
		res, err := m.meta.ReplaceOne(ctx, bson.D{{Key: "_id", Value: key}, {Key: "version", Value: q.Version}}, &next)
		if err != nil {
			return err
		}
		if res.MatchedCount == 0 {
			return ErrConflict
		}
	}
	q.Version = next.Version
	return nil
}

func (m *Mongo) RollSeason(ctx context.Context, to int, tiers []TierMin) error {
	if to < 2 || len(tiers) == 0 {
		return nil
	}
	lp := bson.D{{Key: "$ifNull", Value: bson.A{"$lp", 0}}}
	// The division of the points held at the end of the season.
	branches := bson.A{}
	for i := len(tiers) - 1; i > 0; i-- {
		branches = append(branches, bson.D{{Key: "case", Value: bson.D{{Key: "$gte", Value: bson.A{lp, tiers[i].Min}}}}, {Key: "then", Value: tiers[i].Name}})
	}
	tier := bson.D{{Key: "$switch", Value: bson.D{{Key: "branches", Value: branches}, {Key: "default", Value: tiers[0].Name}}}}
	badge := bson.D{{Key: "season", Value: to - 1}, {Key: "tier", Value: tier}, {Key: "points", Value: lp}}
	played := bson.D{{Key: "$gt", Value: bson.A{bson.D{{Key: "$ifNull", Value: bson.A{"$seasongames", 0}}}, 0}}}
	badges := bson.D{{Key: "$ifNull", Value: bson.A{"$badges", bson.A{}}}}
	update := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: "badges", Value: bson.D{{Key: "$cond", Value: bson.A{played, bson.D{{Key: "$concatArrays", Value: bson.A{badges, bson.A{badge}}}}, badges}}}},
		{Key: "lp", Value: bson.D{{Key: "$toInt", Value: bson.D{{Key: "$floor", Value: bson.D{{Key: "$divide", Value: bson.A{lp, 2}}}}}}}},
		{Key: "seasongames", Value: 0},
		{Key: "season", Value: to},
	}}}}
	filter := bson.D{{Key: "$or", Value: bson.A{
		bson.D{{Key: "season", Value: bson.D{{Key: "$lt", Value: to}}}},
		bson.D{{Key: "season", Value: bson.D{{Key: "$exists", Value: false}}}},
	}}}
	_, err := m.users.UpdateMany(ctx, filter, update)
	return err
}

func (m *Mongo) AddRating(ctx context.Context, userID string, delta int) error {
	// One atomic update: add the points, but never drop below zero.
	lp := bson.D{{Key: "$ifNull", Value: bson.A{"$lp", 0}}}
	games := bson.D{{Key: "$ifNull", Value: bson.A{"$games", 0}}}
	update := mongo.Pipeline{{{Key: "$set", Value: bson.D{
		{Key: "lp", Value: bson.D{{Key: "$max", Value: bson.A{0, bson.D{{Key: "$add", Value: bson.A{lp, delta}}}}}}},
		{Key: "games", Value: bson.D{{Key: "$add", Value: bson.A{games, 1}}}},
		{Key: "seasongames", Value: bson.D{{Key: "$add", Value: bson.A{bson.D{{Key: "$ifNull", Value: bson.A{"$seasongames", 0}}}, 1}}}},
	}}}}
	_, err := m.users.UpdateOne(ctx, id(userID), update)
	return err
}

// visible keeps deleted accounts (and those waiting to be) out of rankings.
var visible = bson.E{Key: "deletedat", Value: bson.D{{Key: "$exists", Value: false}}}

func (m *Mongo) SetDeleted(ctx context.Context, uid string, at time.Time) error {
	update := bson.D{{Key: "$set", Value: bson.D{{Key: "deletedat", Value: at}}}}
	if at.IsZero() {
		update = bson.D{{Key: "$unset", Value: bson.D{{Key: "deletedat", Value: ""}}}}
	}
	res, err := m.users.UpdateOne(ctx, id(uid), update)
	if err == nil && res.MatchedCount == 0 {
		return ErrNotFound
	}
	return err
}

func (m *Mongo) DueForPurge(ctx context.Context, before time.Time, limit int) ([]User, error) {
	cur, err := m.users.Find(ctx, bson.D{
		{Key: "deletedat", Value: bson.D{{Key: "$lt", Value: before}}},
		{Key: "purged", Value: bson.D{{Key: "$ne", Value: true}}},
	}, options.Find().SetLimit(int64(limit)))
	if err != nil {
		return nil, err
	}
	out := []User{}
	err = cur.All(ctx, &out)
	return out, err
}

func (m *Mongo) PurgeUser(ctx context.Context, u *User, anon string) error {
	uid, name := u.ID, u.Name
	steps := []func() error{
		func() error { _, err := m.games.DeleteMany(ctx, bson.D{{Key: "userid", Value: uid}}); return err },
		func() error {
			_, err := m.games.UpdateMany(ctx, bson.D{{Key: "opponent", Value: name}}, bson.D{{Key: "$set", Value: bson.D{{Key: "opponent", Value: anon}}}})
			return err
		},
		func() error {
			_, err := m.games.UpdateMany(ctx, bson.D{{Key: "matches.opponent", Value: name}},
				bson.D{{Key: "$set", Value: bson.D{{Key: "matches.$[m].opponent", Value: anon}}}},
				options.UpdateMany().SetArrayFilters([]any{bson.D{{Key: "m.opponent", Value: name}}}))
			return err
		},
		func() error { _, err := m.runs.DeleteOne(ctx, id(uid)); return err },
		func() error { _, err := m.runs.DeleteOne(ctx, id("daily:"+uid)); return err },
		func() error {
			_, err := m.runs.DeleteMany(ctx, bson.D{{Key: "_id", Value: bson.D{{Key: "$regex", Value: "^vs:.*:" + regexp.QuoteMeta(uid) + "$"}}}})
			return err
		},
		func() error {
			_, err := m.messages.DeleteMany(ctx, bson.D{{Key: "pair", Value: bson.D{{Key: "$regex", Value: "(^|:)" + regexp.QuoteMeta(uid) + "($|:)"}}}})
			return err
		},
		func() error {
			_, err := m.friends.DeleteMany(ctx, bson.D{{Key: "$or", Value: bson.A{bson.D{{Key: "a", Value: uid}}, bson.D{{Key: "b", Value: uid}}}}})
			return err
		},
		func() error {
			_, err := m.feedback.UpdateMany(ctx, bson.D{{Key: "userid", Value: uid}}, bson.D{{Key: "$set", Value: bson.D{{Key: "userid", Value: ""}, {Key: "name", Value: ""}}}})
			return err
		},
		func() error { _, err := m.sessions.DeleteMany(ctx, bson.D{{Key: "userid", Value: uid}}); return err },
		func() error { _, err := m.tokens.DeleteMany(ctx, bson.D{{Key: "userid", Value: uid}}); return err },
		// Last: only once everything else is gone is the account marked purged.
		// Nobody knew a guest by name, so nothing at all is left of one.
		func() error {
			if u.Guest {
				_, err := m.users.DeleteOne(ctx, bson.D{{Key: "_id", Value: uid}, {Key: "guest", Value: true}})
				return err
			}
			stub := User{ID: uid, Name: anon, NameKey: "deleted:" + uid, CreatedAt: u.CreatedAt, DeletedAt: u.DeletedAt, Purged: true}
			_, err := m.users.ReplaceOne(ctx, bson.D{{Key: "_id", Value: uid}, {Key: "purged", Value: bson.D{{Key: "$ne", Value: true}}}}, &stub)
			return err
		},
	}
	for _, step := range steps {
		if err := step(); err != nil {
			return err
		}
	}
	return nil
}

func (m *Mongo) ClaimGuest(ctx context.Context, uid, name, nameKey, passHash, email, emailKey string) error {
	res, err := m.users.UpdateOne(ctx, bson.D{{Key: "_id", Value: uid}, {Key: "guest", Value: true}}, bson.D{
		{Key: "$set", Value: bson.D{{Key: "name", Value: name}, {Key: "namekey", Value: nameKey}, {Key: "passhash", Value: passHash},
			{Key: "email", Value: email}, {Key: "emailkey", Value: emailKey}}},
		{Key: "$unset", Value: bson.D{{Key: "guest", Value: ""}, {Key: "deletedat", Value: ""}}}})
	if err != nil {
		return dupUser(err)
	}
	if res.MatchedCount == 0 {
		return ErrNotFound
	}
	return nil
}

func (m *Mongo) DailyTop(ctx context.Context, day string, n int) ([]GameRecord, error) {
	cur, err := m.games.Find(ctx, bson.D{{Key: "day", Value: day}},
		options.Find().SetSort(bson.D{{Key: "points", Value: -1}, {Key: "gd", Value: -1}, {Key: "at", Value: 1}}).SetLimit(int64(n)))
	if err != nil {
		return nil, err
	}
	out := []GameRecord{}
	err = cur.All(ctx, &out)
	return out, err
}

func (m *Mongo) RankInfo(ctx context.Context, lp int) (int, int, error) {
	above, err := m.users.CountDocuments(ctx, bson.D{{Key: "lp", Value: bson.D{{Key: "$gt", Value: lp}}}, visible})
	if err != nil {
		return 0, 0, err
	}
	total, err := m.users.CountDocuments(ctx, bson.D{visible})
	return int(above), int(total), err
}

func (m *Mongo) TopRated(ctx context.Context, n int) ([]User, error) {
	cur, err := m.users.Find(ctx, bson.D{visible},
		options.Find().SetSort(bson.D{{Key: "lp", Value: -1}, {Key: "name", Value: 1}}).SetLimit(int64(n)))
	if err != nil {
		return nil, err
	}
	var out []User
	if err := cur.All(ctx, &out); err != nil {
		return nil, err
	}
	return out, nil
}
