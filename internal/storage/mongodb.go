package storage

import (
	"context"
	"fmt"
	"log"
	"time"

	"SearchBot/internal/models"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStorage handles MongoDB storage operations
type MongoStorage struct {
	client   *mongo.Client
	database string
	baseCollectionName string
}

// NewMongoStorage creates a new MongoStorage instance
func NewMongoStorage(uri, database, baseCollectionName string) (*MongoStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Ping the database
	err = client.Ping(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	return &MongoStorage{
		client:   client,
		database: database,
		baseCollectionName: baseCollectionName,
	}, nil
}

// getGroupCollection returns the collection for a specific group
func (s *MongoStorage) getGroupCollection(groupID int64) *mongo.Collection {
	collectionName := fmt.Sprintf("%s_group_%d", s.baseCollectionName, groupID)
	collection := s.client.Database(s.database).Collection(collectionName)
	
	// Ensure indexes exist
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	// Create indexes
	indexes := []mongo.IndexModel{
		{
			Keys: bson.D{{Key: "message_id", Value: 1}, {Key: "chat_id", Value: 1}},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.D{{Key: "created_at", Value: -1}},
		},
		{
			Keys: bson.D{{Key: "username", Value: 1}},
		},
	}
	
	_, err := collection.Indexes().CreateMany(ctx, indexes)
	if err != nil {
		log.Printf("Warning: Failed to create indexes: %v", err)
	}
	
	return collection
}

// StoreMessage stores a message in the appropriate group collection
func (s *MongoStorage) StoreMessage(msg *models.Message) error {
	collection := s.getGroupCollection(msg.ChatID)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Check if message already exists
	filter := bson.M{
		"message_id": msg.MessageID,
		"chat_id":    msg.ChatID,
	}
	
	update := bson.M{
		"$set": msg,
	}
	
	opts := options.Update().SetUpsert(true)
	
	result, err := collection.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("failed to store message: %v", err)
	}

	if result.UpsertedCount > 0 {
		log.Printf("Inserted new message: %d", msg.MessageID)
	} else if result.ModifiedCount > 0 {
		log.Printf("Updated existing message: %d", msg.MessageID)
	}

	return nil
}

// GetRecentMessages retrieves recent messages from a specific group
func (s *MongoStorage) GetRecentMessages(groupID int64, limit int64) ([]models.Message, error) {
	collection := s.getGroupCollection(groupID)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	opts := options.Find().
		SetSort(bson.D{{Key: "created_at", Value: -1}}).
		SetLimit(limit)

	cursor, err := collection.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %v", err)
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("failed to decode messages: %v", err)
	}

	return messages, nil
}

// GetMessagesByTimeRange retrieves messages within a time range from a specific group
func (s *MongoStorage) GetMessagesByTimeRange(groupID int64, start, end time.Time) ([]models.Message, error) {
	collection := s.getGroupCollection(groupID)
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"created_at": bson.M{
			"$gte": start,
			"$lte": end,
		},
	}

	opts := options.Find().SetSort(bson.D{{Key: "created_at", Value: 1}})

	cursor, err := collection.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %v", err)
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("failed to decode messages: %v", err)
	}

	return messages, nil
}

func (s *MongoStorage) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func (s *MongoStorage) GetMessagesByChat(chatID int64) ([]*models.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := s.client.Database(s.database).Collection(s.baseCollectionName)
	
	filter := map[string]interface{}{
		"chat_id": chatID,
	}

	cursor, err := collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var messages []*models.Message
	if err = cursor.All(ctx, &messages); err != nil {
		return nil, err
	}

	return messages, nil
} 