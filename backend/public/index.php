<?php

use DI\Container;
use Slim\Factory\AppFactory;
use Tuupola\Middleware\CorsMiddleware;
use App\Controllers\AuthController;
use App\Controllers\ProjectController;
use App\Controllers\TaskController;
use App\Controllers\EventController;
use App\Controllers\AnalyticsController;
use App\Controllers\TeamController;
use App\Middleware\AuthMiddleware;

require __DIR__ . '/../vendor/autoload.php';

// Load environment variables
$dotenv = Dotenv\Dotenv::createImmutable(__DIR__ . '/..');
$dotenv->load();

// Create Container
$container = new Container();
AppFactory::setContainer($container);

// Create App
$app = AppFactory::create();

// Add body parsing middleware
$app->addBodyParsingMiddleware();

// Add CORS middleware
$app->add(new CorsMiddleware([
    "origin" => ["*"],
    "methods" => ["GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"],
    "headers.allow" => ["Authorization", "Content-Type"],
    "headers.expose" => [],
    "credentials" => false,
    "cache" => 0,
]));

// Add error middleware
$app->addErrorMiddleware(true, true, true);

// Add routes
$app->group('/api', function($app) {
    // Auth routes
    $app->post('/login', [AuthController::class, 'login']);
    $app->post('/register', [AuthController::class, 'register']);
    
    // Protected routes
    $app->group('', function($app) {
        $app->get('/me', [AuthController::class, 'me']);
        $app->put('/me', [AuthController::class, 'updateProfile']);

        // Team routes
        $app->get('/team', [TeamController::class, 'getAll']);
        $app->post('/team', [TeamController::class, 'create']);
        $app->put('/team/{id}', [TeamController::class, 'update']);
        $app->delete('/team/{id}', [TeamController::class, 'delete']);

        // Projects routes
        $app->get('/projects', [ProjectController::class, 'getAll']);
        $app->post('/projects', [ProjectController::class, 'create']);
        $app->get('/projects/{id}', [ProjectController::class, 'getById']);
        $app->put('/projects/{id}', [ProjectController::class, 'update']);
        $app->delete('/projects/{id}', [ProjectController::class, 'delete']);
        $app->post('/projects/{id}/team', [ProjectController::class, 'addTeamMember']);
        $app->delete('/projects/{id}/team', [ProjectController::class, 'removeTeamMember']);

        // Tasks routes
        $app->get('/tasks', [TaskController::class, 'getAll']);
        $app->post('/tasks', [TaskController::class, 'create']);
        $app->get('/tasks/{id}', [TaskController::class, 'getById']);
        $app->put('/tasks/{id}', [TaskController::class, 'update']);
        $app->delete('/tasks/{id}', [TaskController::class, 'delete']);
        $app->patch('/tasks/{id}/status', [TaskController::class, 'updateStatus']);
        $app->patch('/tasks/{id}/assign', [TaskController::class, 'assignTask']);
        $app->get('/projects/{projectId}/tasks', [TaskController::class, 'getByProject']);

        // Analytics routes
        $app->get('/analytics/overview', [AnalyticsController::class, 'getOverview']);
        $app->get('/analytics/projects', [AnalyticsController::class, 'getProjectStats']);
        $app->get('/analytics/tasks', [AnalyticsController::class, 'getTaskStats']);
        $app->get('/analytics/team', [AnalyticsController::class, 'getTeamStats']);
        $app->get('/analytics/activity', [AnalyticsController::class, 'getRecentActivity']);

        // Event stream
        $app->get('/events', [EventController::class, 'stream']);
    })->add(new AuthMiddleware());
});

$app->run(); 