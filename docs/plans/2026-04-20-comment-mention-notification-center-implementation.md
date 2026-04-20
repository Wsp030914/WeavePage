# Comment Mention Notification Center Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a comment-driven mention and notification center that keeps comment creation as the main success path while moving mention parsing, in-app notifications, unread aggregation, and email delivery into Kafka-backed async consumers.

**Architecture:** Keep `POST /api/v1/documents/:id/comments` authoritative for comment creation. After a comment is stored, parse only valid structured mentions, publish a `CommentCreated` async event through the existing `IEventBus`, and let separate consumers write notifications, send email, and update unread counters. Invalid mentions must degrade to plain text and must not block comment creation.

**Tech Stack:** Go, Gin, GORM, MySQL, Redis, Kafka, React, Vite, Swagger, go test

---

### Task 1: Define Notification Domain Models And Persistence

**Files:**
- Create: `server/models/notification.go`
- Create: `server/repo/notification_repo.go`
- Modify: `server/initialize/mysql.go`
- Test: `server/repo/notification_repo_test.go`

**Step 1: Write the failing repository tests**

Cover:
- create notification row
- list notifications for one recipient ordered unread-first and recent-first
- mark one notification as read
- mark all notifications as read
- count unread notifications

**Step 2: Run test to verify it fails**

Run: `go test ./server/repo -run TestNotificationRepo -count=1`
Expected: FAIL because `Notification` model and repo do not exist.

**Step 3: Write minimal implementation**

Add:
- `Notification` model with fields:
  - `ID`
  - `RecipientUserID`
  - `ActorUserID`
  - `ProjectID`
  - `TaskID`
  - `CommentID`
  - `Type`
  - `Title`
  - `Body`
  - `Status`
  - `ReadAt`
  - `CreatedAt`
  - `UpdatedAt`
- `NotificationDelivery` model with fields:
  - `ID`
  - `NotificationID`
  - `Channel`
  - `Status`
  - `ErrorMessage`
  - `AttemptCount`
  - `LastAttemptAt`
  - `CreatedAt`
  - `UpdatedAt`
- `UserNotificationPreference` model with fields:
  - `UserID`
  - `CommentNotifyInApp`
  - `CommentNotifyEmail`
  - `MentionNotifyInApp`
  - `MentionNotifyEmail`
  - `CreatedAt`
  - `UpdatedAt`
- repo methods:
  - `Create`
  - `CreateDelivery`
  - `ListByRecipient`
  - `GetUnreadCount`
  - `MarkRead`
  - `MarkAllRead`
  - `GetOrCreatePreference`
  - `UpdatePreference`

**Step 4: Run test to verify it passes**

Run: `go test ./server/repo -run TestNotificationRepo -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add server/models/notification.go server/repo/notification_repo.go server/repo/notification_repo_test.go server/initialize/mysql.go
git commit -m "feat: add notification persistence models"
```

### Task 2: Add Comment Mention Parsing And Event Contract

**Files:**
- Create: `server/async/comment_events.go`
- Create: `server/service/comment_mentions.go`
- Modify: `server/service/task_comment.go`
- Modify: `server/service/task_comment_test.go`

**Step 1: Write the failing service tests**

Cover:
- comment create publishes `CommentCreated` event
- payload contains only valid `mentioned_user_ids`
- invalid mention tokens do not fail comment creation
- users outside current document visibility are excluded

**Step 2: Run test to verify it fails**

Run: `go test ./server/service -run TestTaskCommentServiceCreate -count=1`
Expected: FAIL because mention parser and async publish assertions do not exist.

**Step 3: Write minimal implementation**

Add:
- `CommentCreatedPayload` in `server/async/comment_events.go`
- mention parser in `server/service/comment_mentions.go`
- supported syntax for phase 1:
  - only structured mention tokens such as `@[alice](user:123)`
- parser output:
  - `MentionedUserIDs []int`
  - `InvalidTokens []string`
- update `TaskCommentServiceDeps` to accept:
  - `Bus async.IEventBus`
  - `UserRepo repo.UserRepository`
- in `TaskCommentService.Create`:
  - store comment first
  - parse mentions from saved content
  - validate mentioned user ids exist
  - validate mentioned users are visible to current document session
  - publish `CommentCreated`
  - do not return error for invalid mentions

**Step 4: Run test to verify it passes**

Run: `go test ./server/service -run TestTaskCommentServiceCreate -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add server/async/comment_events.go server/service/comment_mentions.go server/service/task_comment.go server/service/task_comment_test.go
git commit -m "feat: publish comment created events with validated mentions"
```

### Task 3: Implement Notification Projection Consumer

**Files:**
- Create: `server/async/handlers/comment_handlers.go`
- Modify: `server/initialize/events_register.go`
- Modify: `server/async/handlers/deps.go`
- Test: `server/async/handlers/comment_handlers_test.go`

**Step 1: Write the failing consumer tests**

Cover:
- create in-app notification for each valid mention recipient
- skip actor self-notification
- create fallback document-owner notification when no mentions exist
- default notification preference rows are created when absent

**Step 2: Run test to verify it fails**

Run: `go test ./server/async/handlers -run TestHandleCommentCreated -count=1`
Expected: FAIL because comment handler is not registered.

**Step 3: Write minimal implementation**

Add `HandleCommentCreated` consumer:
- input: `CommentCreatedPayload`
- resolve recipients:
  - mentioned users first
  - if no mention recipients, notify document owner and eligible collaborators
- apply rules:
  - never notify actor
  - never notify users not visible to the document
  - skip duplicate recipients
- write `notifications`
- write `notification_deliveries` rows with channel `in_app` and status `sent`

Register:
- `consumer.Register("CommentCreated", handlers.HandleCommentCreated)`

**Step 4: Run test to verify it passes**

Run: `go test ./server/async/handlers -run TestHandleCommentCreated -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add server/async/handlers/comment_handlers.go server/async/handlers/comment_handlers_test.go server/initialize/events_register.go server/async/handlers/deps.go
git commit -m "feat: project comment events into notifications"
```

### Task 4: Implement Email Delivery Consumer

**Files:**
- Modify: `server/async/handlers/comment_handlers.go`
- Modify: `server/async/handlers/comment_handlers_test.go`
- Modify: `server/initialize/events_register.go`

**Step 1: Write the failing email consumer tests**

Cover:
- send email only when recipient preference enables comment or mention email
- create `notification_deliveries` rows with `sent`, `failed`, or `skipped`
- do not fail notification projection when one email delivery fails

**Step 2: Run test to verify it fails**

Run: `go test ./server/async/handlers -run TestHandleCommentEmailDelivery -count=1`
Expected: FAIL because email delivery consumer does not exist.

**Step 3: Write minimal implementation**

Add `HandleCommentNotificationEmail` consumer:
- input: `CommentCreatedPayload`
- fetch notification rows created for the comment
- apply user preference:
  - mention email for mentioned recipients
  - comment email for non-mentioned recipients
- write delivery rows:
  - `pending`
  - `sent`
  - `failed`
  - `skipped`
- keep email failures isolated per recipient

Register:
- `consumer.Register("CommentCreatedEmail", handlers.HandleCommentNotificationEmail)`

Publish strategy:
- after `CommentCreated` projection succeeds, publish a follow-up async job `CommentCreatedEmail`
- keep this as a separate consumer to demonstrate multi-consumer Kafka value

**Step 4: Run test to verify it passes**

Run: `go test ./server/async/handlers -run TestHandleCommentEmailDelivery -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add server/async/handlers/comment_handlers.go server/async/handlers/comment_handlers_test.go server/initialize/events_register.go
git commit -m "feat: add comment notification email delivery consumer"
```

### Task 5: Add Notification Service And HTTP APIs

**Files:**
- Create: `server/service/notification_service.go`
- Create: `server/handler/notification.go`
- Modify: `server/router.go`
- Modify: `server/main.go`
- Test: `server/service/notification_service_test.go`
- Test: `server/handler/notification_test.go`

**Step 1: Write the failing service and handler tests**

Cover:
- list notifications for current user
- unread count endpoint
- mark single notification as read
- mark all notifications as read
- get and update preferences
- reject access to another user’s notifications

**Step 2: Run test to verify it fails**

Run: `go test ./server/service -run TestNotificationService -count=1`
Run: `go test ./server/handler -run TestNotificationHandler -count=1`
Expected: FAIL because notification service and handler do not exist.

**Step 3: Write minimal implementation**

Add endpoints:
- `GET /api/v1/notifications`
- `GET /api/v1/notifications/unread-count`
- `POST /api/v1/notifications/:id/read`
- `POST /api/v1/notifications/read-all`
- `GET /api/v1/notification-preferences`
- `PATCH /api/v1/notification-preferences`

Wire:
- build `NotificationService` in `server/main.go`
- inject repo dependencies
- register routes in `server/router.go`

**Step 4: Run test to verify it passes**

Run: `go test ./server/service -run TestNotificationService -count=1`
Run: `go test ./server/handler -run TestNotificationHandler -count=1`
Expected: PASS

**Step 5: Commit**

```bash
git add server/service/notification_service.go server/handler/notification.go server/service/notification_service_test.go server/handler/notification_test.go server/router.go server/main.go
git commit -m "feat: add notification center APIs"
```

### Task 6: Add Frontend Notification Center

**Files:**
- Create: `web/src/api/notification.js`
- Create: `web/src/pages/NotificationsPage.jsx`
- Modify: `web/src/layouts/AppLayout.jsx`
- Modify: `web/src/App.jsx`
- Test: `web/src/pages/NotificationsPage.jsx` via manual QA

**Step 1: Write the failing integration checklist**

Cover:
- sidebar or top-nav entry opens notifications page
- unread badge is visible
- notification list loads current user rows
- single notification can be marked read
- mark-all-read updates unread badge
- preferences page section can toggle comment and mention email/in-app settings

**Step 2: Run existing frontend checks to confirm no notification entry exists**

Run: `cd web && npm run build`
Expected: PASS, but no notification UI exists.

**Step 3: Write minimal implementation**

Add API client wrappers:
- `getNotifications`
- `getNotificationUnreadCount`
- `markNotificationRead`
- `markAllNotificationsRead`
- `getNotificationPreferences`
- `updateNotificationPreferences`

Add page:
- unread-first list
- mark-read action
- mark-all action
- preference toggles

Update layout:
- add navigation entry and unread badge

**Step 4: Run checks to verify it passes**

Run: `cd web && npm run build`
Expected: PASS

Manual QA:
- create comment mentioning another user
- open notifications page as recipient
- unread badge increments
- mark read decrements unread badge

**Step 5: Commit**

```bash
git add web/src/api/notification.js web/src/pages/NotificationsPage.jsx web/src/layouts/AppLayout.jsx web/src/App.jsx
git commit -m "feat: add notification center frontend"
```

### Task 7: Extend Swagger And Project Docs

**Files:**
- Modify: `docs/swagger.yaml`
- Modify: `docs/swagger.json`
- Modify: `docs/docs.go`
- Modify: `README.md`
- Modify: `AGENTS.md`
- Modify: `server/AGENTS.md`

**Step 1: Write the failing doc checklist**

Cover:
- notification endpoints documented
- mention behavior documented
- invalid mentions described as plain-text degradation
- Kafka role updated from generic async side effects to comment notification multi-consumer example

**Step 2: Regenerate or verify docs currently miss notification APIs**

Run: inspect generated swagger for `/notifications`
Expected: missing

**Step 3: Write minimal implementation**

Update:
- Swagger comments on notification handlers
- generated swagger files
- root docs describing:
  - comment-created event
  - mention parsing rules
  - notification center scope

**Step 4: Run checks to verify it passes**

Run: `go test ./...`
Run: `cd web && npm run build`
Expected: PASS

**Step 5: Commit**

```bash
git add docs/swagger.yaml docs/swagger.json docs/docs.go README.md AGENTS.md server/AGENTS.md
git commit -m "docs: document notification center and comment events"
```

### Task 8: Final End-To-End Verification

**Files:**
- Test: `server/service/task_comment_test.go`
- Test: `server/async/handlers/comment_handlers_test.go`
- Test: `server/service/notification_service_test.go`
- Test: `server/handler/notification_test.go`

**Step 1: Add missing regression cases**

Cover:
- invalid mention does not fail comment creation
- actor self-mention does not create notification
- duplicate mentions create only one notification
- preference-off skips email but keeps in-app notification
- mark-all-read only affects current user

**Step 2: Run focused tests**

Run: `go test ./server/service -run "TestTaskCommentServiceCreate|TestNotificationService" -count=1`
Run: `go test ./server/async/handlers -run "TestHandleCommentCreated|TestHandleCommentEmailDelivery" -count=1`
Run: `go test ./server/handler -run TestNotificationHandler -count=1`
Expected: PASS

**Step 3: Run full backend suite**

Run: `go test ./...`
Expected: PASS

**Step 4: Run frontend build and manual flow**

Run: `cd web && npm run build`
Expected: PASS

Manual flow:
- user A comments with valid mention to user B
- user A comments with invalid mention token
- user B sees unread notification
- user B marks it read
- email delivery row is visible for valid recipient only

**Step 5: Commit**

```bash
git add .
git commit -m "test: verify comment mention notification flow"
```

