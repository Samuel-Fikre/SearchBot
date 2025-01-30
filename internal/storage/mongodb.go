package storage

import (
	"context"
	"log"
	"time"

	"SearchBot/internal/models"

	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type MongoStorage struct {
	client     *mongo.Client
	database   string
	collection string
}

func NewMongoStorage(uri, database, collection string) (*MongoStorage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	client, err := mongo.Connect(ctx, options.Client().ApplyURI(uri))
	if err != nil {
		return nil, err
	}

	// Ping the database
	if err := client.Ping(ctx, nil); err != nil {
		return nil, err
	}

	return &MongoStorage{
		client:     client,
		database:   database,
		collection: collection,
	}, nil
}

func (s *MongoStorage) Close() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return s.client.Disconnect(ctx)
}

func (s *MongoStorage) StoreMessage(msg *models.Message) error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := s.client.Database(s.database).Collection(s.collection)
	
	_, err := collection.InsertOne(ctx, msg)
	if err != nil {
		log.Printf("Error storing message: %v", err)
		return err
	}

	return nil
}

func (s *MongoStorage) GetMessagesByChat(chatID int64) ([]*models.Message, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	collection := s.client.Database(s.database).Collection(s.collection)
	
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