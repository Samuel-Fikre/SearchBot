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

// MongoDB implements MessageStorage interface
type MongoDB struct {
	client             *mongo.Client
	database           string
	baseCollectionName string
}

// NewMongoDB creates a new MongoDB instance
func NewMongoDB(uri, database, baseCollectionName string) (*MongoDB, error) {
	// Create client options with longer timeouts for Atlas
	clientOptions := options.Client().
		ApplyURI(uri).
		SetServerSelectionTimeout(30 * time.Second).
		SetConnectTimeout(30 * time.Second).
		SetSocketTimeout(30 * time.Second)

	log.Printf("Attempting to connect to MongoDB...")

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Connect to MongoDB
	client, err := mongo.Connect(ctx, clientOptions)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to MongoDB: %v", err)
	}

	// Ping the database
	if err := client.Ping(ctx, nil); err != nil {
		// Try to disconnect if ping fails
		if disconnectErr := client.Disconnect(ctx); disconnectErr != nil {
			log.Printf("Warning: Failed to disconnect after ping failure: %v", disconnectErr)
		}
		return nil, fmt.Errorf("failed to ping MongoDB: %v", err)
	}

	log.Printf("Successfully connected to MongoDB")

	return &MongoDB{
		client:             client,
		database:           database,
		baseCollectionName: baseCollectionName,
	}, nil
}

// getGroupCollection returns the collection for a specific group
func (s *MongoDB) getGroupCollection(chatID int64) *mongo.Collection {
	collectionName := fmt.Sprintf("%s_group_%d", s.baseCollectionName, chatID)
	return s.client.Database(s.database).Collection(collectionName)
}

// StoreMessage stores a message in MongoDB
func (s *MongoDB) StoreMessage(msg *models.Message) error {
	collection := s.getGroupCollection(msg.ChatID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Try to update existing message
	filter := bson.M{
		"message_id": msg.MessageID,
		"chat_id":    msg.ChatID,
	}

	update := bson.M{"$set": msg}
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

// GetMessagesByChat retrieves messages for a specific chat
func (s *MongoDB) GetMessagesByChat(chatID int64) ([]models.Message, error) {
	collection := s.getGroupCollection(chatID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cursor, err := collection.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch messages: %v", err)
	}
	defer cursor.Close(ctx)

	var messages []models.Message
	if err := cursor.All(ctx, &messages); err != nil {
		return nil, fmt.Errorf("failed to decode messages: %v", err)
	}

	return messages, nil
}

// GetMessage retrieves a specific message
func (s *MongoDB) GetMessage(chatID int64, messageID int64) (*models.Message, error) {
	collection := s.getGroupCollection(chatID)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	filter := bson.M{
		"message_id": messageID,
		"chat_id":    chatID,
	}

	var message models.Message
	err := collection.FindOne(ctx, filter).Decode(&message)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch message: %v", err)
	}

	return &message, nil
}

func (s *MongoDB) Close() error {
	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.client.Disconnect(ctx)
	}
	return nil
}

// GetRecentMessages retrieves recent messages from a specific group
func (s *MongoDB) GetRecentMessages(groupID int64, limit int64) ([]models.Message, error) {
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
func (s *MongoDB) GetMessagesByTimeRange(groupID int64, start, end time.Time) ([]models.Message, error) {
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
