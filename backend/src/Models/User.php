<?php

namespace App\Models;

use App\Config\Database;
use MongoDB\BSON\ObjectId;

class User {
    private $collection;

    public function __construct() {
        $this->collection = Database::getInstance()->getCollection($_ENV['USERS_COLLECTION']);
    }

    public function findByEmail($email) {
        return $this->collection->findOne(['email' => $email]);
    }

    public function create($data) {
        $data['created_at'] = new \MongoDB\BSON\UTCDateTime();
        $result = $this->collection->insertOne($data);
        return $this->findById($result->getInsertedId());
    }

    public function update($id, $data) {
        if (isset($data['password'])) {
            $data['password'] = password_hash($data['password'], PASSWORD_DEFAULT);
        }
        $data['updated_at'] = new \MongoDB\BSON\UTCDateTime();
        $this->collection->updateOne(
            ['_id' => new ObjectId($id)],
            ['$set' => $data]
        );
        return $this->findById($id);
    }

    public function findById($id) {
        return $this->collection->findOne(['_id' => new ObjectId($id)]);
    }

    public function getAll() {
        return $this->collection->find()->toArray();
    }

    public function delete($id) {
        return $this->collection->deleteOne(['_id' => new ObjectId($id)]);
    }
} 