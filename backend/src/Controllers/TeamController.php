<?php

namespace App\Controllers;

use Psr\Http\Message\ResponseInterface as Response;
use Psr\Http\Message\ServerRequestInterface as Request;
use App\Models\User;
use App\Models\Project;
use MongoDB\BSON\ObjectId;

class TeamController {
    private $user;
    private $project;

    public function __construct() {
        $this->user = new User();
        $this->project = new Project();
    }

    public function getAll(Request $request, Response $response): Response {
        $users = $this->user->getAll();
        $projects = $this->project->getAll();

        $members = [];
        foreach ($users as $user) {
            $activeProjects = 0;
            $completedProjects = 0;

            foreach ($projects as $project) {
                if (!isset($project->team)) continue;
                
                $isTeamMember = in_array(
                    (string)$user->_id, 
                    array_map(fn($m) => (string)$m, $project->team)
                );
                
                if ($isTeamMember) {
                    if ($project->status === 'completed') {
                        $completedProjects++;
                    } else {
                        $activeProjects++;
                    }
                }
            }

            $members[] = [
                'id' => (string)$user->_id,
                'name' => $user->name,
                'email' => $user->email,
                'role' => $user->role,
                'phone' => $user->phone ?? '',
                'avatar' => substr($user->name, 0, 2),
                'activeProjects' => $activeProjects,
                'completedProjects' => $completedProjects
            ];
        }

        $response->getBody()->write(json_encode(['members' => $members]));
        return $response->withHeader('Content-Type', 'application/json');
    }

    public function create(Request $request, Response $response): Response {
        $data = $request->getParsedBody();
        
        if (empty($data['name']) || empty($data['email']) || empty($data['role'])) {
            $response->getBody()->write(json_encode([
                'error' => 'Name, email and role are required'
            ]));
            return $response
                ->withHeader('Content-Type', 'application/json')
                ->withStatus(400);
        }

        if ($this->user->findByEmail($data['email'])) {
            $response->getBody()->write(json_encode([
                'error' => 'Email already exists'
            ]));
            return $response
                ->withHeader('Content-Type', 'application/json')
                ->withStatus(400);
        }

        $user = $this->user->create([
            'name' => $data['name'],
            'email' => $data['email'],
            'role' => $data['role'],
            'phone' => $data['phone'] ?? '',
            'password' => password_hash(uniqid(), PASSWORD_DEFAULT), // Generate random password
        ]);

        $member = [
            'id' => (string)$user->_id,
            'name' => $user->name,
            'email' => $user->email,
            'role' => $user->role,
            'phone' => $user->phone ?? '',
            'avatar' => substr($user->name, 0, 2),
            'activeProjects' => 0,
            'completedProjects' => 0
        ];

        $response->getBody()->write(json_encode(['member' => $member]));
        return $response->withHeader('Content-Type', 'application/json');
    }

    public function update(Request $request, Response $response, array $args): Response {
        $data = $request->getParsedBody();
        
        if (empty($data)) {
            $response->getBody()->write(json_encode([
                'error' => 'No data to update'
            ]));
            return $response
                ->withHeader('Content-Type', 'application/json')
                ->withStatus(400);
        }

        $user = $this->user->findById($args['id']);
        if (!$user) {
            $response->getBody()->write(json_encode([
                'error' => 'Member not found'
            ]));
            return $response
                ->withHeader('Content-Type', 'application/json')
                ->withStatus(404);
        }

        if (isset($data['email']) && $data['email'] !== $user->email) {
            if ($this->user->findByEmail($data['email'])) {
                $response->getBody()->write(json_encode([
                    'error' => 'Email already exists'
                ]));
                return $response
                    ->withHeader('Content-Type', 'application/json')
                    ->withStatus(400);
            }
        }

        $updatedUser = $this->user->update($args['id'], $data);
        $projects = $this->project->getAll();

        $activeProjects = 0;
        $completedProjects = 0;
        foreach ($projects as $project) {
            if (!isset($project->team)) continue;
            
            $isTeamMember = in_array(
                (string)$updatedUser->_id, 
                array_map(fn($m) => (string)$m, $project->team)
            );
            
            if ($isTeamMember) {
                if ($project->status === 'completed') {
                    $completedProjects++;
                } else {
                    $activeProjects++;
                }
            }
        }

        $member = [
            'id' => (string)$updatedUser->_id,
            'name' => $updatedUser->name,
            'email' => $updatedUser->email,
            'role' => $updatedUser->role,
            'phone' => $updatedUser->phone ?? '',
            'avatar' => substr($updatedUser->name, 0, 2),
            'activeProjects' => $activeProjects,
            'completedProjects' => $completedProjects
        ];

        $response->getBody()->write(json_encode(['member' => $member]));
        return $response->withHeader('Content-Type', 'application/json');
    }

    public function delete(Request $request, Response $response, array $args): Response {
        $user = $this->user->findById($args['id']);
        if (!$user) {
            $response->getBody()->write(json_encode([
                'error' => 'Member not found'
            ]));
            return $response
                ->withHeader('Content-Type', 'application/json')
                ->withStatus(404);
        }

        // Remove user from all projects
        $projects = $this->project->getAll();
        foreach ($projects as $project) {
            if (isset($project->team) && in_array($user->_id, $project->team)) {
                $this->project->removeTeamMember((string)$project->_id, $user->_id);
            }
        }

        // Delete the user
        $this->user->delete($args['id']);

        $response->getBody()->write(json_encode([
            'message' => 'Member deleted successfully'
        ]));
        return $response->withHeader('Content-Type', 'application/json');
    }
} 