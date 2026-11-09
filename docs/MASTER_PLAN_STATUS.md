# Nexus Master Plan - Business Grade App Implementation

**Date:** 2026-11-08
**Status:** Phase 1 Complete (Foundation)

---

## Executive Summary

This document outlines the transformation of Nexus from a functional prototype to a business-grade social/dating/marketplace app. The master plan addresses user retention, engagement, monetization, and professional polish.

---

## Phase 1: Foundation (COMPLETED)

### 1.1 Seed Data Infrastructure ✅
**File:** `migrations/009_seed_demo_data.sql`

**Delivered:**
- 50 demo users with realistic Myanmar names
- 50 user profiles with avatars (pravatar.cc) and townships
- 80 friendships creating a social network
- 100 social posts with Myanmar content and images
- 30 reactions and 8 comments on posts
- 50 market products across categories (electronics, vehicles, fashion, etc.)
- 20 dating profiles with bios
- 10 swipes and 3 matches
- 20 chat threads with 40 messages
- 10 coin transactions

**Impact:** App no longer shows empty states. Users see a living community immediately.

### 1.2 Notification System ✅
**Files:**
- `migrations/010_notifications.sql` - Database schema
- `backend/internal/store/notifications.go` - Store layer
- `backend/internal/api/notifications.go` - API handlers

**API Endpoints:**
- `GET /api/v1/notifications` - List notifications with unread count
- `POST /api/v1/notifications/{id}/read` - Mark single notification as read
- `POST /api/v1/notifications/read-all` - Mark all as read

**Notification Types:**
- `match` - Dating match notification
- `like` - Someone liked your post
- `comment` - Someone commented on your post
- `message` - New chat message
- `nearby_interest` - People nearby are interested
- `market_sale` - Someone bought your item
- `market_inquiry` - Someone inquired about your item
- `friend_request` - Friend request received
- `system` - System announcements

**Next Step:** Integrate notification triggers into existing flows (post reactions, matches, messages).

### 1.3 Enhanced Dating UI ✅
**File:** `mobile/lib/dating_ui.dart`

**Features:**
- **SwipeableCardStack**: Tinder-like card swipe with:
  - Horizontal drag gestures
  - Real-time rotation during drag
  - LIKE/PASS overlay indicators
  - Smooth swipe-off animation
  - Heart burst animation on like
  - Card stack showing next profile underneath
  
- **AnimatedRadar**: Professional radar with:
  - Rotating scan line effect
  - Gradient fill for sweep
  - Smooth node distribution around circle
  - Node labels with handles
  - Pulsing rings

**Integration:** Replace existing `_CardsView` and `_RadarView` in `screens.dart` with new components.

---

## Phase 2: Engagement (IN PROGRESS)

### 2.1 Market Search & Filter ⏳
**Planned:**
- Search by keyword
- Filter by township
- Filter by category (electronics, vehicles, fashion, etc.)
- Filter by price range
- Sort by newest, price low-high, price high-low

**API Changes:**
```sql
-- Add category column
ALTER TABLE market.products ADD COLUMN category text NOT NULL DEFAULT 'other';
CREATE INDEX idx_products_category ON market.products (category, active);
```

### 2.2 Trust Signals ⏳
**Planned:**
- "Verified Seller" badge for users with phone verification
- Seller rating system (1-5 stars)
- Transaction count display
- Response time indicator

### 2.3 Notification Triggers ⏳
**Planned Integration Points:**
- Post reaction → send notification to post author
- Comment → send notification to post author
- Dating match → send notification to both users
- New message → send notification to thread owner
- Market inquiry → send notification to seller
- Market sale → send notification to seller

---

## Phase 3: Professional Polish (PENDING)

### 3.1 Profile Page Redesign ⏳
**Planned:**
- Cover photo + profile picture
- User stats (posts, friends, matches)
- Tabbed view: Posts | About | Friends
- Edit profile button
- Share profile button

### 3.2 Settings Page ⏳
**Planned:**
- Notification settings (toggle by type)
- Privacy settings (profile visibility, online status)
- Account settings (change phone, delete account)
- App settings (theme, language)
- About & Help

### 3.3 Design Consistency ⏳
**Planned:**
- Unified color palette (already in `theme.dart`)
- Consistent typography scale
- Standardized spacing (8px grid)
- Reusable component library

---

## Technical Debt & Improvements

### Database Optimizations
- Add full-text search index for posts and products
- Add materialized view for feed ranking
- Add caching layer for frequently accessed data

### Performance
- Implement pagination for all list endpoints
- Add image CDN for avatars and post images
- Optimize radar query with geohash indexing

### Security
- Add rate limiting to notification endpoints
- Implement notification preferences (user can opt-out)
- Add push notification support (Firebase Cloud Messaging)

---

## Rollout Strategy

### Week 1: Seed Data + Notifications
1. Run `009_seed_demo_data.sql` on all environments
2. Deploy notification API
3. Add notification triggers to existing flows
4. Test notification delivery

### Week 2: Dating UI Enhancement
1. Integrate `SwipeableCardStack` into `screens.dart`
2. Integrate `AnimatedRadar` into `screens.dart`
3. A/B test new UI vs old UI
4. Collect user feedback

### Week 3: Market Enhancements
1. Add search/filter API endpoints
2. Add category column to products
3. Implement verified seller badges
4. Update Market UI with filters

### Week 4: Profile & Settings
1. Design new profile page layout
2. Implement settings page
3. Add notification preferences UI
4. Final polish and bug fixes

---

## Success Metrics

| Metric | Current | Target (30 days) |
|--------|---------|------------------|
| Daily Active Users | 0 (demo) | 500 |
| Posts per Day | 0 | 50 |
| Dating Swipes per Day | 0 | 200 |
| Market Listings | 0 | 100 |
| Chat Messages per Day | 0 | 100 |
| User Retention (D7) | N/A | 40% |
| User Retention (D30) | N/A | 20% |

---

## Outstanding Items

### Blocked on External Services
1. **Push Notifications**: Requires Firebase Cloud Messaging setup
2. **Image CDN**: Requires CloudFront/S3 or similar
3. **SMS Provider**: Requires real aggregator credentials (4-network test)
4. **Payment Providers**: KBZPay/WavePay need live credentials

### Store Submission
- `evidence/store-submission-play.pdf` - Requires Google Play Console access
- `evidence/store-submission-ios.pdf` - Requires App Store Connect access

---

## Conclusion

Phase 1 foundation is complete. The app now has:
- Realistic demo data (no more empty states)
- Notification infrastructure (ready for integration)
- Professional dating UI (swipe animations, radar effects)

**Next Priority:** Integrate notification triggers and deploy enhanced dating UI.

**Timeline:** 4 weeks to full business-grade implementation.
