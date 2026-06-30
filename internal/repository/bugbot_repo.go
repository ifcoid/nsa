package repository

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo/options"

	"nsa/internal/model"
)

const (
	bugTicketsCollection = "bug_tickets"
	bugBotStateCollection = "bug_bot_state"
)

// GetPollOffset retrieves the current Telegram getUpdates offset from bug_bot_state.
// Returns 0 if no state document exists yet.
func (r *MongoRepository) GetPollOffset(ctx context.Context) (int64, error) {
	coll := r.client.Database(r.dbName).Collection(bugBotStateCollection)

	var state model.BugBotPollState
	err := coll.FindOne(ctx, bson.M{"_id": "poll_state"}).Decode(&state)
	if err != nil {
		// If document not found, return 0 (start from beginning)
		if err.Error() == "mongo: no documents in result" {
			return 0, nil
		}
		return 0, err
	}
	return state.Offset, nil
}

// SetPollOffset updates the Telegram getUpdates offset in bug_bot_state (upsert).
func (r *MongoRepository) SetPollOffset(ctx context.Context, offset int64) error {
	coll := r.client.Database(r.dbName).Collection(bugBotStateCollection)

	opts := options.Update().SetUpsert(true)
	_, err := coll.UpdateOne(ctx, bson.M{"_id": "poll_state"},
		bson.M{"$set": bson.M{"offset": offset}}, opts)
	return err
}

// CreateBugTicket inserts a new bug ticket into the bug_tickets collection.
func (r *MongoRepository) CreateBugTicket(ctx context.Context, ticket *model.BugTicket) error {
	coll := r.client.Database(r.dbName).Collection(bugTicketsCollection)

	if ticket.ID.IsZero() {
		ticket.ID = primitive.NewObjectID()
	}
	now := time.Now()
	ticket.CreatedAt = now
	ticket.UpdatedAt = now
	if ticket.Status == "" {
		ticket.Status = "open"
	}
	if ticket.Replies == nil {
		ticket.Replies = []model.BugTicketReply{}
	}

	_, err := coll.InsertOne(ctx, ticket)
	return err
}

// GetUnresolvedTickets returns all bug tickets where status is NOT "closed" or "resolved".
func (r *MongoRepository) GetUnresolvedTickets(ctx context.Context) ([]model.BugTicket, error) {
	coll := r.client.Database(r.dbName).Collection(bugTicketsCollection)

	filter := bson.M{
		"status": bson.M{"$nin": []string{"closed", "resolved"}},
	}
	opts := options.Find().SetSort(bson.M{"created_at": -1})

	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var tickets []model.BugTicket
	if err := cursor.All(ctx, &tickets); err != nil {
		return nil, err
	}
	return tickets, nil
}

// GetTicketByID retrieves a single bug ticket by its ObjectID (hex string).
func (r *MongoRepository) GetTicketByID(ctx context.Context, id string) (*model.BugTicket, error) {
	coll := r.client.Database(r.dbName).Collection(bugTicketsCollection)

	objID, err := primitive.ObjectIDFromHex(id)
	if err != nil {
		return nil, err
	}

	var ticket model.BugTicket
	err = coll.FindOne(ctx, bson.M{"_id": objID}).Decode(&ticket)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// GetLastTicket retrieves the most recently created bug ticket.
func (r *MongoRepository) GetLastTicket(ctx context.Context) (*model.BugTicket, error) {
	coll := r.client.Database(r.dbName).Collection(bugTicketsCollection)

	opts := options.FindOne().SetSort(bson.M{"created_at": -1})

	var ticket model.BugTicket
	err := coll.FindOne(ctx, bson.M{}, opts).Decode(&ticket)
	if err != nil {
		return nil, err
	}
	return &ticket, nil
}

// AddReplyToTicket appends a reply to the ticket's replies array and updates updated_at.
// If the ticket is currently "open", it also sets status to "in_progress".
func (r *MongoRepository) AddReplyToTicket(ctx context.Context, ticketID string, reply model.BugTicketReply) error {
	coll := r.client.Database(r.dbName).Collection(bugTicketsCollection)

	objID, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return err
	}

	now := time.Now()
	update := bson.M{
		"$push": bson.M{"replies": reply},
		"$set":  bson.M{"updated_at": now},
	}

	// First, get the ticket to check its status
	var ticket model.BugTicket
	err = coll.FindOne(ctx, bson.M{"_id": objID}).Decode(&ticket)
	if err != nil {
		return err
	}

	// If currently "open", advance to "in_progress"
	if ticket.Status == "open" {
		update["$set"] = bson.M{
			"updated_at": now,
			"status":     "in_progress",
		}
	}

	_, err = coll.UpdateOne(ctx, bson.M{"_id": objID}, update)
	return err
}

// UpdateTicketStatus sets a ticket's status. If status is "resolved", also sets resolved_at.
func (r *MongoRepository) UpdateTicketStatus(ctx context.Context, ticketID string, status string) error {
	coll := r.client.Database(r.dbName).Collection(bugTicketsCollection)

	objID, err := primitive.ObjectIDFromHex(ticketID)
	if err != nil {
		return err
	}

	now := time.Now()
	setFields := bson.M{
		"status":     status,
		"updated_at": now,
	}
	if status == "resolved" {
		setFields["resolved_at"] = now
	}

	_, err = coll.UpdateOne(ctx, bson.M{"_id": objID}, bson.M{"$set": setFields})
	return err
}
