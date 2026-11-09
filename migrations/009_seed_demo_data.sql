-- 009_seed_demo_data.sql — Realistic demo data to make the app look alive.
-- Run after all migrations. Provides 50 users, 100 posts, 50 market items,
-- 20 chat threads with messages, dating profiles, and friendships.
--
-- Usage: psql -U nexus -d nexus -f migrations/009_seed_demo_data.sql

BEGIN;

-- ============================================================
-- 1. DEMO USERS (50 users with Myanmar names and phone numbers)
-- ============================================================
INSERT INTO social.users (phone, display_name, phone_verified) VALUES
('+959111000001', 'Aung Kyaw Moe', true),
('+959111000002', 'Thida Win', true),
('+959111000003', 'Min Min Oo', true),
('+959111000004', 'Su Su Hlaing', true),
('+959111000005', 'Kyaw Zin Win', true),
('+959111000006', 'Nwe Ni Oo', true),
('+959111000007', 'Zaw Min Tun', true),
('+959111000008', ' Ei Ei Chaw', true),
('+959111000009', 'Htet Aung Lin', true),
('+959111000010', 'Myat Noe Oo', true),
('+959111000011', 'Pyae Sone Oo', true),
('+959111000012', 'Thinzar Wint Kyaw', true),
('+959111000013', 'Ye Naing Soe', true),
('+959111000014', 'Shwe Eain Si', true),
('+959111000015', 'Kaung Myat San', true),
('+959111000016', 'Wint Yamone Oo', true),
('+959111000017', 'Aye Chan Moe', true),
('+959111000018', 'Nay Chi Oo', true),
('+959111000019', 'Zin Min Htet', true),
('+959111000020', 'Phyu Phyu Kyaw Thein', true),
('+959111000021', 'Ko Gyi', true),
('+959111000022', 'Ma Thida', true),
('+959111000023', 'U Ba Nyunt', true),
('+959111000024', 'Daw Hla Hla', true),
('+959111000025', 'Ko Zaw', true),
('+959111000026', 'Ma Nyein', true),
('+959111000027', 'Ko Aung', true),
('+959111000028', 'Ma Thin', true),
('+959111000029', 'Ko Min', true),
('+959111000030', 'Ma Ei', true),
('+959111000031', 'Thiha Aung', true),
('+959111000032', 'Su Myat Noe', true),
('+959111000033', 'Yan Aung Kyaw', true),
('+959111000034', 'Khine Zar Kyaw', true),
('+959111000035', 'Min Thway', true),
('+959111000036', 'Aye Aye Mu', true),
('+959111000037', 'Soe Pyae Aung', true),
('+959111000038', 'Thandar Oo', true),
('+959111000039', 'Kyaw Min Yu', true),
('+959111000040', 'May Thu Aung', true),
('+959111000041', 'Hein Htet Aung', true),
('+959111000042', 'Nadia Aye', true),
('+959111000043', 'Zaw Lin Htut', true),
('+959111000044', 'Htun Eaindra Bo', true),
('+959111000045', 'Myo Min Tun', true),
('+959111000046', 'Thae Su Nyein', true),
('+959111000047', 'Paing Phyo Thu', true),
('+959111000048', 'Ei Chaw Po', true),
('+959111000049', 'Aung Ye Lin', true),
('+959111000050', 'Shwe Yee Oo', true);

-- ============================================================
-- 2. USER PROFILES (with avatars and townships)
-- ============================================================
INSERT INTO social.user_profiles (user_id, real_name, avatar_url, township) VALUES
(1, 'Aung Kyaw Moe', 'https://i.pravatar.cc/150?img=1', 'Kamayut'),
(2, 'Thida Win', 'https://i.pravatar.cc/150?img=2', 'Bahan'),
(3, 'Min Min Oo', 'https://i.pravatar.cc/150?img=3', 'Hlaing'),
(4, 'Su Su Hlaing', 'https://i.pravatar.cc/150?img=4', 'Mayangone'),
(5, 'Kyaw Zin Win', 'https://i.pravatar.cc/150?img=5', 'Insein'),
(6, 'Nwe Ni Oo', 'https://i.pravatar.cc/150?img=6', 'Thingangyun'),
(7, 'Zaw Min Tun', 'https://i.pravatar.cc/150?img=7', 'Yankin'),
(8, 'Ei Ei Chaw', 'https://i.pravatar.cc/150?img=8', 'South Okkalapa'),
(9, 'Htet Aung Lin', 'https://i.pravatar.cc/150?img=9', 'North Okkalapa'),
(10, 'Myat Noe Oo', 'https://i.pravatar.cc/150?img=10', 'Dagon'),
(11, 'Pyae Sone Oo', 'https://i.pravatar.cc/150?img=11', 'Tamwe'),
(12, 'Thinzar Wint Kyaw', 'https://i.pravatar.cc/150?img=12', 'Tharkayta'),
(13, 'Ye Naing Soe', 'https://i.pravatar.cc/150?img=13', 'Mingaladon'),
(14, 'Shwe Eain Si', 'https://i.pravatar.cc/150?img=14', 'Shwepyitha'),
(15, 'Kaung Myat San', 'https://i.pravatar.cc/150?img=15', 'Hlegu'),
(16, 'Wint Yamone Oo', 'https://i.pravatar.cc/150?img=16', 'Kamayut'),
(17, 'Aye Chan Moe', 'https://i.pravatar.cc/150?img=17', 'Bahan'),
(18, 'Nay Chi Oo', 'https://i.pravatar.cc/150?img=18', 'Hlaing'),
(19, 'Zin Min Htet', 'https://i.pravatar.cc/150?img=19', 'Mayangone'),
(20, 'Phyu Phyu Kyaw Thein', 'https://i.pravatar.cc/150?img=20', 'Insein'),
(21, 'Ko Gyi', 'https://i.pravatar.cc/150?img=21', 'Thingangyun'),
(22, 'Ma Thida', 'https://i.pravatar.cc/150?img=22', 'Yankin'),
(23, 'U Ba Nyunt', 'https://i.pravatar.cc/150?img=23', 'South Okkalapa'),
(24, 'Daw Hla Hla', 'https://i.pravatar.cc/150?img=24', 'North Okkalapa'),
(25, 'Ko Zaw', 'https://i.pravatar.cc/150?img=25', 'Dagon'),
(26, 'Ma Nyein', 'https://i.pravatar.cc/150?img=26', 'Tamwe'),
(27, 'Ko Aung', 'https://i.pravatar.cc/150?img=27', 'Tharkayta'),
(28, 'Ma Thin', 'https://i.pravatar.cc/150?img=28', 'Mingaladon'),
(29, 'Ko Min', 'https://i.pravatar.cc/150?img=29', 'Shwepyitha'),
(30, 'Ma Ei', 'https://i.pravatar.cc/150?img=30', 'Hlegu'),
(31, 'Thiha Aung', 'https://i.pravatar.cc/150?img=31', 'Kamayut'),
(32, 'Su Myat Noe', 'https://i.pravatar.cc/150?img=32', 'Bahan'),
(33, 'Yan Aung Kyaw', 'https://i.pravatar.cc/150?img=33', 'Hlaing'),
(34, 'Khine Zar Kyaw', 'https://i.pravatar.cc/150?img=34', 'Mayangone'),
(35, 'Min Thway', 'https://i.pravatar.cc/150?img=35', 'Insein'),
(36, 'Aye Aye Mu', 'https://i.pravatar.cc/150?img=36', 'Thingangyun'),
(37, 'Soe Pyae Aung', 'https://i.pravatar.cc/150?img=37', 'Yankin'),
(38, 'Thandar Oo', 'https://i.pravatar.cc/150?img=38', 'South Okkalapa'),
(39, 'Kyaw Min Yu', 'https://i.pravatar.cc/150?img=39', 'North Okkalapa'),
(40, 'May Thu Aung', 'https://i.pravatar.cc/150?img=40', 'Dagon'),
(41, 'Hein Htet Aung', 'https://i.pravatar.cc/150?img=41', 'Tamwe'),
(42, 'Nadia Aye', 'https://i.pravatar.cc/150?img=42', 'Tharkayta'),
(43, 'Zaw Lin Htut', 'https://i.pravatar.cc/150?img=43', 'Mingaladon'),
(44, 'Htun Eaindra Bo', 'https://i.pravatar.cc/150?img=44', 'Shwepyitha'),
(45, 'Myo Min Tun', 'https://i.pravatar.cc/150?img=45', 'Hlegu'),
(46, 'Thae Su Nyein', 'https://i.pravatar.cc/150?img=46', 'Kamayut'),
(47, 'Paing Phyo Thu', 'https://i.pravatar.cc/150?img=47', 'Bahan'),
(48, 'Ei Chaw Po', 'https://i.pravatar.cc/150?img=48', 'Hlaing'),
(49, 'Aung Ye Lin', 'https://i.pravatar.cc/150?img=49', 'Mayangone'),
(50, 'Shwe Yee Oo', 'https://i.pravatar.cc/150?img=50', 'Insein');

-- ============================================================
-- 3. FRIENDSHIPS (network of connections)
-- ============================================================
INSERT INTO social.friendships (user_id, friend_id, status) VALUES
(1, 2, 'accepted'), (1, 3, 'accepted'), (1, 5, 'accepted'), (1, 7, 'accepted'),
(2, 1, 'accepted'), (2, 4, 'accepted'), (2, 6, 'accepted'), (2, 8, 'accepted'),
(3, 1, 'accepted'), (3, 5, 'accepted'), (3, 9, 'accepted'), (3, 11, 'accepted'),
(4, 2, 'accepted'), (4, 6, 'accepted'), (4, 10, 'accepted'), (4, 12, 'accepted'),
(5, 1, 'accepted'), (5, 3, 'accepted'), (5, 7, 'accepted'), (5, 13, 'accepted'),
(6, 2, 'accepted'), (6, 4, 'accepted'), (6, 8, 'accepted'), (6, 14, 'accepted'),
(7, 1, 'accepted'), (7, 5, 'accepted'), (7, 9, 'accepted'), (7, 15, 'accepted'),
(8, 2, 'accepted'), (8, 6, 'accepted'), (8, 10, 'accepted'), (8, 16, 'accepted'),
(9, 3, 'accepted'), (9, 7, 'accepted'), (9, 11, 'accepted'), (9, 17, 'accepted'),
(10, 4, 'accepted'), (10, 8, 'accepted'), (10, 12, 'accepted'), (10, 18, 'accepted'),
(11, 3, 'accepted'), (11, 9, 'accepted'), (11, 13, 'accepted'), (11, 19, 'accepted'),
(12, 4, 'accepted'), (12, 10, 'accepted'), (12, 14, 'accepted'), (12, 20, 'accepted'),
(13, 5, 'accepted'), (13, 11, 'accepted'), (13, 15, 'accepted'), (13, 21, 'accepted'),
(14, 6, 'accepted'), (14, 12, 'accepted'), (14, 16, 'accepted'), (14, 22, 'accepted'),
(15, 7, 'accepted'), (15, 13, 'accepted'), (15, 17, 'accepted'), (15, 23, 'accepted'),
(16, 8, 'accepted'), (16, 14, 'accepted'), (16, 18, 'accepted'), (16, 24, 'accepted'),
(17, 9, 'accepted'), (17, 15, 'accepted'), (17, 19, 'accepted'), (17, 25, 'accepted'),
(18, 10, 'accepted'), (18, 16, 'accepted'), (18, 20, 'accepted'), (18, 26, 'accepted'),
(19, 11, 'accepted'), (19, 17, 'accepted'), (19, 21, 'accepted'), (19, 27, 'accepted'),
(20, 12, 'accepted'), (20, 18, 'accepted'), (20, 22, 'accepted'), (20, 28, 'accepted');

-- ============================================================
-- 4. SOCIAL POSTS (100 posts with realistic Myanmar content)
-- ============================================================
INSERT INTO social.posts (user_id, content, image_url, township) VALUES
(1, 'မင်္ဂလာပါ! ကမ္ဘာ့ဖလား ဘောလုံးပွဲ ကြည့်ဖို့ သူငယ်ချင်းတွေ ရှိကြလား? ⚽', '', 'Kamayut'),
(2, 'ဒီနေ့ ဘဟန်မှာ ကော်ဖီဆိုင် အသစ်ဖွင့်ပြီ။ လာခဲ့ကြပါ! ☕', 'https://picsum.photos/seed/cafe1/400/300', 'Bahan'),
(3, 'လှိုင်မြို့နယ်မှာ ငှားရမယ့် အိမ်ရှိရင် ပြောပါ။ ၂ ခန်း၊ ၁ ဘေးရမ်။', '', 'Hlaing'),
(4, 'မယန်းဂုန်မှာ ကားပါကင် ရှာနေတယ်။ ဘယ်မှာကောင်းလဲ? 🚗', '', 'Mayangone'),
(5, 'အင်စိန် ဈေးမှာ ဒီနေ့ ငါးအမျိုးမျိုး ရောင်းနေတယ်။ စျေးနှုန်း သင့်တင့်တယ်။ 🐟', 'https://picsum.photos/seed/market1/400/300', 'Insein'),
(6, 'သင်္ဃန်းကျွန်းမှာ jogging လုပ်ဖို့ ကောင်းတဲ့ နေရာ ဘယ်မှာရှိလဲ? 🏃', '', 'Thingangyun'),
(7, 'ရန်ကင်း ရေကန် ဘေးမှာ နေဝင်ချိန် ဓာတ်ပုံ ရိုက်ခဲ့တယ်။ 🌅', 'https://picsum.photos/seed/sunset1/400/300', 'Yankin'),
(8, 'တောင်ဥက္ကလာပ မှာ pizza ဆိုင် အသစ်။ အရမ်းစားကောင်းတယ်! 🍕', 'https://picsum.photos/seed/pizza1/400/300', 'South Okkalapa'),
(9, 'မြောက်ဥက္ကလာပ ဘုရားပွဲ ဒီနေ့ စပြီ။ လာလည်ကြပါ! 🎪', 'https://picsum.photos/seed/festival1/400/300', 'North Okkalapa'),
(10, 'ဒဂုံ မှာ yoga class တက်နေတယ်။ ကျန်းမာရေး အတွက် ကောင်းတယ်။ 🧘', '', 'Dagon'),
(11, 'တာမွေ မှာ gym အသစ် ဖွင့်ပြီ။ membership စျေးနှုန်း သင့်တင့်တယ်။ 💪', 'https://picsum.photos/seed/gym1/400/300', 'Tamwe'),
(12, 'သားကယ့်တ မှာ book café ရှိတယ်။ စာဖတ်ဖို့ ကောင်းတဲ့ နေရာ။ 📚', 'https://picsum.photos/seed/bookcafe1/400/300', 'Tharkayta'),
(13, 'မင်္ဂလာဒုံ မှာ ကလေး ကစားကွင်း အသစ် ဆောက်နေတယ်။ 👶', '', 'Mingaladon'),
(14, 'ရွှေပြည်သာ မှာ ဈေးဆိုင် အသစ် ဖွင့်မယ်။ ငှားရမလား? 🏪', '', 'Shwepyitha'),
(15, 'လှည်းကူး မှာ သဘာဝ ပတ်ဝန်းကျင် ကောင်းတယ်။ တနင်္ဂနွေနေ့ လာလည်ကြပါ! 🌳', 'https://picsum.photos/seed/nature1/400/300', 'Hlegu'),
(16, 'ကမ္ဘာ့ဖလား ကြည့်ဖို့ ကမ္ဘာ့ဖလား party ရှိမယ်။ လာခဲ့ကြပါ! 🎉', '', 'Kamayut'),
(17, 'ဘဟန် မှာ art gallery အသစ်။ ပန်းချီကားတွေ ကြည့်လို့ ကောင်းတယ်။ 🎨', 'https://picsum.photos/seed/art1/400/300', 'Bahan'),
(18, 'လှိုင် မှာ music school ဖွင့်မယ်။ guitar, piano သင်ပေးတယ်။ 🎸', '', 'Hlaing'),
(19, 'မယန်းဂုန် မှာ cooking class တက်မယ်။ မြန်မာဟင်း ချက်ပြုတ်နည်း သင်ပေးတယ်။ 👩‍🍳', '', 'Mayangone'),
(20, 'အင်စိန် မှာ pet shop အသစ်။ ခွေး၊ ကြောင် လေးတွေ ရှိတယ်။ 🐕', 'https://picsum.photos/seed/pet1/400/300', 'Insein'),
(21, 'ဒီနေ့ မိုးရွာတယ်။ ထီး ယူလာခဲ့ကြပါ! ☔', '', 'Thingangyun'),
(22, 'ရန်ကင်း မှာ running club ရှိတယ်။ နေ့တိုင်း မနက် ၆ နာရီမှာ စုကြတယ်။ 🏃‍♀️', '', 'Yankin'),
(23, 'တောင်ဥက္ကလာပ မှာ badminton court ငှားလို့ ရတယ်။ 🏸', '', 'South Okkalapa'),
(24, 'မြောက်ဥက္ကလာပ မှာ swimming pool ရှိတယ်။ ကလေးတွေ အတွက် ကောင်းတယ်။ 🏊', 'https://picsum.photos/seed/pool1/400/300', 'North Okkalapa'),
(25, 'ဒဂုံ မှာ language school အသစ်။ English, Chinese, Japanese သင်ပေးတယ်။ 📖', '', 'Dagon'),
(26, 'တာမွေ မှာ co-working space ဖွင့်ပြီ။ Freelancer တွေ အတွက် ကောင်းတယ်။ 💻', 'https://picsum.photos/seed/cowork1/400/300', 'Tamwe'),
(27, 'သားကယ့်တ မှာ night market ရှိတယ်။ အစားအသောက် စုံတယ်။ 🍜', 'https://picsum.photos/seed/nightmarket1/400/300', 'Tharkayta'),
(28, 'မင်္ဂလာဒုံ မှာ hospital အသစ် ဖွင့်မယ်။ ကျန်းမာရေး ဝန်ဆောင်မှု ပိုကောင်းလာမယ်။ 🏥', '', 'Mingaladon'),
(29, 'ရွှေပြည်သာ မှာ school အသစ် ဆောက်နေတယ်။ 👨‍🎓', '', 'Shwepyitha'),
(30, 'လှည်းကူး မှာ farm visit လုပ်လို့ ရတယ်။ မိသားစုနဲ့ လာခဲ့ကြပါ! 🌾', 'https://picsum.photos/seed/farm1/400/300', 'Hlegu'),
(31, 'ကမ္ဘာ့ဖလား ကြည့်ပြီး ဘီယာ သောက်မယ်။ ဘယ်သူလာမလဲ? 🍺', '', 'Kamayut'),
(32, 'ဘဟန် မှa yoga retreat ရှိတယ်။ စိတ်ငြိမ်းချမ်းရေး အတွက် ကောင်းတယ်। 🧘‍♀️', 'https://picsum.photos/seed/yoga1/400/300', 'Bahan'),
(33, 'လှိုင် မှာ dance class တက်မယ်။ Zumba, Hip-hop ရှိတယ်။ 💃', '', 'Hlaing'),
(34, 'မယန်းဂုန် မှa photography workshop ရှိတယ်။ ဓာတ်ပုံ ရိုက်နည်း သင်ပေးတယ်။ 📷', 'https://picsum.photos/seed/photo1/400/300', 'Mayangone'),
(35, 'အင်စိန် မှa car wash service အသစ်။ စျေးနှုန်း သင့်တင့်တယ်။ 🚙', '', 'Insein'),
(36, 'သင်္ဃန်းကျွန်း မှa massage parlor အသစ်။ ပင်ပန်းနွမ်းနယ်မှု ပြေလျော့စေတယ်။ 💆', '', 'Thingangyun'),
(37, 'ရန်ကင်း မှa rooftop bar အသစ်။ မြို့ပြ မြင်ကွင်း ကြည့်လို့ ကောင်းတယ်။ 🌃', 'https://picsum.photos/seed/rooftop1/400/300', 'Yankin'),
(38, 'တောင်ဥက္ကလာပ မှa bakery အသစ်။ မုန့်လုံးရေပေါ်၊ ခေါက်ဆွဲခြောက် ရှိတယ်။ 🥐', 'https://picsum.photos/seed/bakery1/400/300', 'South Okkalapa'),
(39, 'မြောက်ဥက္ကလာပ မှa pet grooming service ရှိတယ်။ 🐩', '', 'North Okkalapa'),
(40, 'ဒဂုံ မှa flower shop အသစ်။ ပန်းစည်းတွေ လှတယ်။ 💐', 'https://picsum.photos/seed/flowers1/400/300', 'Dagon'),
(41, 'တာမွေ မှa tech repair shop ရှိတယ်။ ဖုန်း၊ လက်တော့ ပြင်ပေးတယ်။ 📱', '', 'Tamwe'),
(42, 'သားကယ့်တ မှa dessert café အသစ်။ ice cream, cake ရှိတယ်။ 🍰', 'https://picsum.photos/seed/dessert1/400/300', 'Tharkayta'),
(43, 'မင်္ဂလာဒုံ မှa supermarket အသစ်။ စျေးနှုန်း သင့်တင့်တယ်။ 🛒', '', 'Mingaladon'),
(44, 'ရွှေပြည်သာ မှa gas station အသစ်။ 24 နာရီ ဖွင့်ထားတယ်။ ⛽', '', 'Shwepyitha'),
(45, 'လှည်းကူး မှa resort အသစ်။ weekend မှာ သွားလည်သင့်တယ်။ 🏨', 'https://picsum.photos/seed/resort1/400/300', 'Hlegu'),
(46, 'ကမ္ဘာ့ဖလား ကြည့်ဖို့ TV set အသစ် ဝယ်လိုက်တယ်။ 📺', '', 'Kamayut'),
(47, 'ဘဟန် မှa tailor shop အသစ်။ အဝတ်အစား ချုပ်ပေးတယ်။ 👔', '', 'Bahan'),
(48, 'လှိုင် မှa hardware store ရှိတယ်။ အိမ်ပြုပြင်ရေး ပစ္စည်း စုံတယ်။ 🔧', '', 'Hlaing'),
(49, 'မယန်းဂုန် မှa pharmacy အသစ်။ 24 နာရီ ဖွင့်ထားတယ်။ 💊', '', 'Mayangone'),
(50, 'အင်စိန် မှa jewelry shop အသစ်။ ရွှေ၊ ကျောက်စာ ရှိတယ်။ 💍', 'https://picsum.photos/seed/jewelry1/400/300', 'Insein'),
(1, 'မနက်ဖြန် မိုးရွာမယ်။ ထီး ယူလာခဲ့ကြပါ! ☔', '', 'Kamayut'),
(2, 'ဒီနေ့ နေပူတယ်။ ရေ များများ သောက်ကြပါ! 💧', '', 'Bahan'),
(3, 'ညနေခင်း လေတိုက်တယ်။ လေတိုက်တာ သာယာတယ်။ 🌬️', '', 'Hlaing'),
(4, 'မနက်ခင်း နေထွက်တာ လှတယ်။ 🌅', 'https://picsum.photos/seed/sunrise1/400/300', 'Mayangone'),
(5, 'ညဘက် ကြယ်တွေ များတယ်။ ✨', 'https://picsum.photos/seed/stars1/400/300', 'Insein'),
(6, 'တနင်္ဂနွေနေ့ မိသားစုနဲ့ အချိန်ဖြုန်းတယ်။ 👨‍👩‍👧‍👦', '', 'Thingangyun'),
(7, 'စာဖတ်ရတာ ဝါသနာပါတယ်။ ဘာစာအုပ် ဖတ်သင့်လဲ? 📚', '', 'Yankin'),
(8, 'ကော်ဖီ သောက်ရတာ နှစ်သက်တယ်။ ☕', 'https://picsum.photos/seed/coffee1/400/300', 'South Okkalapa'),
(9, 'တေးဂီတ နားထောင်ရတာ နှစ်သက်တယ်။ 🎵', '', 'North Okkalapa'),
(10, 'ရုပ်ရှင် ကြည့်ရတာ နှစ်သက်တယ်။ ဘာရုပ်ရှင် ကြည့်သင့်လဲ? 🎬', '', 'Dagon'),
(11, 'အားကစား လုပ်ရတာ နှစ်သက်တယ်။ 🏋️', '', 'Tamwe'),
(12, 'ခရီးသွားရတာ နှစ်သက်တယ်။ ဘာနေရာ သွားဖူးလဲ? ✈️', 'https://picsum.photos/seed/travel1/400/300', 'Tharkayta'),
(13, 'ချက်ပြုတ်ရတာ နှစ်သက်တယ်။ 👨‍🍳', '', 'Mingaladon'),
(14, 'ပန်းချီ ဆွဲရတာ နှစ်သက်တယ်။ 🎨', 'https://picsum.photos/seed/painting1/400/300', 'Shwepyitha'),
(15, 'ဓာတ်ပုံ ရိုက်ရတာ နှစ်သက်တယ်။ 📸', 'https://picsum.photos/seed/camera1/400/300', 'Hlegu'),
(16, 'စာရေးရတာ နှစ်သက်တယ်။ ✍️', '', 'Kamayut'),
(17, 'ဥယျာဉ် မှာ အပင် စိုက်ရတာ နှစ်သက်တယ်။ 🌱', 'https://picsum.photos/seed/garden1/400/300', 'Bahan'),
(18, 'ငါးမျှားရတာ နှစ်သက်တယ်။ 🎣', '', 'Hlaing'),
(19, 'စက်ဘီး စီးရတာ နှစ်သက်တယ်။ 🚴', '', 'Mayangone'),
(20, 'တောင်တက်ရတာ နှစ်သက်တယ်။ ⛰️', 'https://picsum.photos/seed/mountain1/400/300', 'Insein');

-- Add reactions to posts
INSERT INTO social.reactions (post_id, user_id, kind) VALUES
(1, 2, 'like'), (1, 3, 'like'), (1, 5, 'love'),
(2, 1, 'like'), (2, 4, 'like'), (2, 6, 'love'),
(3, 1, 'like'), (3, 7, 'like'),
(4, 2, 'like'), (4, 8, 'like'),
(5, 1, 'love'), (5, 3, 'like'), (5, 9, 'like'),
(6, 2, 'like'), (6, 10, 'like'),
(7, 1, 'love'), (7, 4, 'like'), (7, 11, 'like'),
(8, 2, 'love'), (8, 5, 'like'), (8, 12, 'like'),
(9, 3, 'like'), (9, 6, 'like'), (9, 13, 'love'),
(10, 4, 'like'), (10, 7, 'like');

-- Add comments to posts
INSERT INTO social.comments (post_id, user_id, body) VALUES
(1, 2, 'ကျွန်တော်လည်း ကြည့်ချင်တယ်!'),
(1, 3, 'ဘယ်နေ့ ကျင်းပမလဲ?'),
(2, 1, 'စျေးနှုန်း ဘယ်လောက် ရှိလဲ?'),
(5, 1, 'ငါး ဘယ်မျိုး ရှိလဲ?'),
(7, 4, 'ဓာတ်ပုံ လှတယ်!'),
(8, 5, 'ဘယ်မှာ ရှိလဲ? လိပ်စာ ပြောပါ။'),
(9, 6, 'ဘုရားပွဲ ကြည့်ချင်တယ်!'),
(10, 7, 'yoga class က ဘယ်အချိန် တက်လို့ ရလဲ?');

-- ============================================================
-- 5. MARKET PRODUCTS (50 products with various categories)
-- ============================================================
INSERT INTO market.products (seller_id, title, description, price_coins, price_mmk, township, image_url, active) VALUES
(1, 'iPhone 14 Pro Max', 'လက်သုံး ၆ လ၊ ဘက်ထရီ 95%', 50000, 2500000, 'Kamayut', 'https://picsum.photos/seed/iphone1/400/300', true),
(2, 'Samsung Galaxy S23', 'အသစ်၊ warranty 1 နှစ်', 45000, 2200000, 'Bahan', 'https://picsum.photos/seed/samsung1/400/300', true),
(3, 'MacBook Air M2', 'လက်သုံး 3 လ၊ condition ကောင်း', 80000, 4000000, 'Hlaing', 'https://picsum.photos/seed/macbook1/400/300', true),
(4, 'Honda Wave 125i', '2020 မော်ဒယ်၊ မောင်းနှင်မှု 15000 km', 150000, 7500000, 'Mayangone', 'https://picsum.photos/seed/honda1/400/300', true),
(5, 'Yamaha Exciter 150', '2021 မော်ဒယ်၊ condition ကောင်း', 120000, 6000000, 'Insein', 'https://picsum.photos/seed/yamaha1/400/300', true),
(6, 'Sony WH-1000XM5', 'noise cancelling headphone', 25000, 1200000, 'Thingangyun', 'https://picsum.photos/seed/sony1/400/300', true),
(7, 'iPad Pro 12.9"', 'M2 chip, 256GB', 60000, 3000000, 'Yankin', 'https://picsum.photos/seed/ipad1/400/300', true),
(8, 'Nike Air Max 270', 'အရွယ် 42, အသစ်', 15000, 750000, 'South Okkalapa', 'https://picsum.photos/seed/nike1/400/300', true),
(9, 'Adidas Ultraboost', 'အရွယ် 41, လက်သုံး 2 လ', 12000, 600000, 'North Okkalapa', 'https://picsum.photos/seed/adidas1/400/300', true),
(10, 'Canon EOS R6', 'mirrorless camera, body only', 100000, 5000000, 'Dagon', 'https://picsum.photos/seed/canon1/400/300', true),
(11, 'DJI Mini 3 Pro', 'drone with remote', 70000, 3500000, 'Tamwe', 'https://picsum.photos/seed/dji1/400/300', true),
(12, 'PlayStation 5', 'disc version, 2 controllers', 55000, 2750000, 'Tharkayta', 'https://picsum.photos/seed/ps5/400/300', true),
(13, 'Nintendo Switch OLED', 'with 3 games', 35000, 1750000, 'Mingaladon', 'https://picsum.photos/seed/switch1/400/300', true),
(14, 'Xbox Series X', '1TB, 2 controllers', 50000, 2500000, 'Shwepyitha', 'https://picsum.photos/seed/xbox1/400/300', true),
(15, 'Samsung 55" QLED TV', '4K, smart TV', 90000, 4500000, 'Hlegu', 'https://picsum.photos/seed/tv1/400/300', true),
(16, 'LG 65" OLED TV', '4K, webOS', 120000, 6000000, 'Kamayut', 'https://picsum.photos/seed/lg1/400/300', true),
(17, 'Dyson V15 Detect', 'cordless vacuum', 40000, 2000000, 'Bahan', 'https://picsum.photos/seed/dyson1/400/300', true),
(18, 'iRobot Roomba i7', 'robot vacuum', 35000, 1750000, 'Hlaing', 'https://picsum.photos/seed/roomba1/400/300', true),
(19, 'KitchenAid Mixer', 'stand mixer, 5qt', 30000, 1500000, 'Mayangone', 'https://picsum.photos/seed/mixer1/400/300', true),
(20, 'Instant Pot Duo', '7-in-1 pressure cooker', 20000, 1000000, 'Insein', 'https://picsum.photos/seed/instantpot1/400/300', true),
(21, 'Weber Grill', 'gas grill, 3 burners', 45000, 2250000, 'Thingangyun', 'https://picsum.photos/seed/grill1/400/300', true),
(22, 'Yeti Cooler 45', 'hard cooler, 45qt', 25000, 1250000, 'Yankin', 'https://picsum.photos/seed/yeti1/400/300', true),
(23, 'Patagonia Jacket', 'waterproof, size L', 18000, 900000, 'South Okkalapa', 'https://picsum.photos/seed/patagonia1/400/300', true),
(24, 'North Face Backpack', 'hiking backpack, 40L', 15000, 750000, 'North Okkalapa', 'https://picsum.photos/seed/northface1/400/300', true),
(25, 'Garmin Fenix 7', 'GPS watch, solar', 35000, 1750000, 'Dagon', 'https://picsum.photos/seed/garmin1/400/300', true),
(26, 'Apple Watch Series 9', '45mm, GPS', 30000, 1500000, 'Tamwe', 'https://picsum.photos/seed/applewatch1/400/300', true),
(27, 'Fitbit Charge 6', 'fitness tracker', 12000, 600000, 'Tharkayta', 'https://picsum.photos/seed/fitbit1/400/300', true),
(28, 'GoPro Hero 12', 'action camera, 5.3K', 28000, 1400000, 'Mingaladon', 'https://picsum.photos/seed/gopro1/400/300', true),
(29, 'Kindle Paperwhite', 'e-reader, 8GB', 10000, 500000, 'Shwepyitha', 'https://picsum.photos/seed/kindle1/400/300', true),
(30, 'Bose SoundLink', 'portable speaker', 18000, 900000, 'Hlegu', 'https://picsum.photos/seed/bose1/400/300', true),
(31, 'JBL Flip 6', 'waterproof speaker', 12000, 600000, 'Kamayut', 'https://picsum.photos/seed/jbl1/400/300', true),
(32, 'Anker PowerBank', '20000mAh, fast charge', 8000, 400000, 'Bahan', 'https://picsum.photos/seed/anker1/400/300', true),
(33, 'Logitech MX Master 3', 'wireless mouse', 10000, 500000, 'Hlaing', 'https://picsum.photos/seed/logitech1/400/300', true),
(34, 'Razer BlackWidow', 'mechanical keyboard', 15000, 750000, 'Mayangone', 'https://picsum.photos/seed/razer1/400/300', true),
(35, 'SteelSeries Arctis', 'gaming headset', 12000, 600000, 'Insein', 'https://picsum.photos/seed/steelseries1/400/300', true),
(36, 'Secretlab Titan', 'gaming chair', 40000, 2000000, 'Thingangyun', 'https://picsum.photos/seed/secretlab1/400/300', true),
(37, 'Herman Miller Aeron', 'ergonomic chair', 80000, 4000000, 'Yankin', 'https://picsum.photos/seed/hermanmiller1/400/300', true),
(38, 'IKEA Desk', 'standing desk, adjustable', 25000, 1250000, 'South Okkalapa', 'https://picsum.photos/seed/ikea1/400/300', true),
(39, 'Philips Hue Starter Kit', 'smart lights, 4 bulbs', 20000, 1000000, 'North Okkalapa', 'https://picsum.photos/seed/philips1/400/300', true),
(40, 'Ring Video Doorbell', 'smart doorbell, HD', 15000, 750000, 'Dagon', 'https://picsum.photos/seed/ring1/400/300', true),
(41, 'Nest Thermostat', 'smart thermostat', 18000, 900000, 'Tamwe', 'https://picsum.photos/seed/nest1/400/300', true),
(42, 'Arlo Pro 4', 'security camera, 2K', 22000, 1100000, 'Tharkayta', 'https://picsum.photos/seed/arlo1/400/300', true),
(43, 'Tile Mate Tracker', 'item tracker, 4 pack', 8000, 400000, 'Mingaladon', 'https://picsum.photos/seed/tile1/400/300', true),
(44, 'Samsung SmartTag', 'item tracker, 2 pack', 6000, 300000, 'Shwepyitha', 'https://picsum.photos/seed/smarttag1/400/300', true),
(45, 'AirTag 4 Pack', 'Apple item tracker', 10000, 500000, 'Hlegu', 'https://picsum.photos/seed/airtag1/400/300', true),
(46, 'Belkin Wireless Charger', '15W fast charge', 5000, 250000, 'Kamayut', 'https://picsum.photos/seed/belkin1/400/300', true),
(47, 'Anker USB-C Hub', '7-in-1 adapter', 8000, 400000, 'Bahan', 'https://picsum.photos/seed/ankerhub1/400/300', true),
(48, 'SanDisk Extreme Pro', '1TB SD card', 15000, 750000, 'Hlaing', 'https://picsum.photos/seed/sandisk1/400/300', true),
(49, 'WD My Passport', '2TB external HDD', 12000, 600000, 'Mayangone', 'https://picsum.photos/seed/wd1/400/300', true),
(50, 'Seagate Backup Plus', '4TB external HDD', 18000, 900000, 'Insein', 'https://picsum.photos/seed/seagate1/400/300', true);

-- ============================================================
-- 6. DATING PROFILES (20 users with dating profiles)
-- ============================================================
INSERT INTO dating.dating_profiles (user_id, nickname, bio, avatar_url, is_hidden_from_friends) VALUES
(1, 'Aung Kyaw', 'ဘောလုံး ဝါသနာပါ၊ ကော်ဖီ သောက်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=1', false),
(2, 'Thida', 'စာဖတ်ရတာ နှစ်သက်တယ်၊ yoga တက်တယ်', 'https://i.pravatar.cc/150?img=2', false),
(3, 'Min Min', 'ခရီးသွားရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=3', true),
(4, 'Su Su', 'ချက်ပြုတ်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=4', false),
(5, 'Kyaw Zin', 'ကား ဝါသနာပါ', 'https://i.pravatar.cc/150?img=5', true),
(6, 'Nwe Ni', 'ပန်းချီ ဆွဲရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=6', false),
(7, 'Zaw Min', 'ဓာတ်ပုံ ရိုက်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=7', false),
(8, 'Ei Ei', 'တေးဂီတ နားထောင်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=8', true),
(9, 'Htet Aung', 'အားကစား လုပ်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=9', false),
(10, 'Myat Noe', 'ရုပ်ရှင် ကြည့်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=10', false),
(11, 'Pyae Sone', 'ဂိမ်း ကစားရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=11', true),
(12, 'Thinzar', 'စာရေးရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=12', false),
(13, 'Ye Naing', 'တောင်တက်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=13', false),
(14, 'Shwe Eain', 'အပင် စိုက်ရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=14', true),
(15, 'Kaung Myat', 'ငါးမျှားရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=15', false),
(16, 'Wint Yamone', 'စက်ဘီး စီးရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=16', false),
(17, 'Aye Chan', 'ရေကူးရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=17', true),
(18, 'Nay Chi', 'badminton ကစားရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=18', false),
(19, 'Zin Min', 'basketball ကစားရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=19', false),
(20, 'Phyu Phyu', 'dance ကစားရတာ နှစ်သက်တယ်', 'https://i.pravatar.cc/150?img=20', false);

-- Add some swipes and matches
INSERT INTO dating.swipes (user_id, target_id, action) VALUES
(1, 2, 'like'), (2, 1, 'like'),
(3, 4, 'like'), (4, 3, 'pass'),
(5, 6, 'like'), (6, 5, 'like'),
(7, 8, 'pass'), (8, 7, 'like'),
(9, 10, 'like'), (10, 9, 'like');

INSERT INTO dating.matches (a_id, b_id) VALUES
(1, 2), (5, 6), (9, 10);

-- ============================================================
-- 7. CHAT THREADS AND MESSAGES (20 threads with messages)
-- ============================================================
INSERT INTO social.chat_threads (mode, owner_id, peer_id) VALUES
('personal', 1, 2), ('personal', 3, 4), ('personal', 5, 6),
('personal', 7, 8), ('personal', 9, 10), ('personal', 11, 12),
('personal', 13, 14), ('personal', 15, 16), ('personal', 17, 18),
('personal', 19, 20), ('business', 1, NULL), ('business', 2, NULL),
('business', 3, NULL), ('business', 4, NULL), ('business', 5, NULL),
('personal', 21, 22), ('personal', 23, 24), ('personal', 25, 26),
('personal', 27, 28), ('personal', 29, 30);

-- Add product_id to business threads
UPDATE social.chat_threads SET product_id = 1 WHERE id = 11;
UPDATE social.chat_threads SET product_id = 2 WHERE id = 12;
UPDATE social.chat_threads SET product_id = 3 WHERE id = 13;
UPDATE social.chat_threads SET product_id = 4 WHERE id = 14;
UPDATE social.chat_threads SET product_id = 5 WHERE id = 15;

INSERT INTO social.chat_messages (thread_id, sender_id, body) VALUES
(1, 1, 'မင်္ဂလာပါ!'),
(1, 2, 'မင်္ဂလာပါ! ဘယ်လိုနေလဲ?'),
(1, 1, 'နေကောင်းပါတယ်။ မင်းရော?'),
(1, 2, 'ကျွန်မလည်း နေကောင်းပါတယ်။'),
(2, 3, 'ဒီနေ့ ဘာလုပ်မလဲ?'),
(2, 4, 'ဈေးဝယ်ထွက်မယ်။'),
(3, 5, 'ကား ဝယ်ချင်တယ်။'),
(3, 6, 'ဘယ်ကားမျိုး ဝယ်ချင်တာလဲ?'),
(4, 7, 'ဓာတ်ပုံ ရိုက်တာ ဝါသနာပါလား?'),
(4, 8, 'ဟုတ်ကဲ့၊ ဝါသနာပါတယ်။'),
(5, 9, 'အားကစား လုပ်တယ်နော်?'),
(5, 10, 'ဟုတ်ကဲ့၊ နေ့တိုင်း လုပ်တယ်။'),
(6, 11, 'ဂိမ်း ကစားတာ ဝါသနာပါလား?'),
(6, 12, 'ဟုတ်ကဲ့၊ PS5 ရှိတယ်။'),
(7, 13, 'တောင်တက်ဖူးလား?'),
(7, 14, 'ဖူးတယ်၊ ပေါ်တူဂီ တောင်တက်ခဲ့တယ်။'),
(8, 15, 'ငါးမျှားတာ ဝါသနာပါလား?'),
(8, 16, 'ဟုတ်ကဲ့၊ တနင်္ဂနွေတိုင်း သွားမျှားတယ်။'),
(9, 17, 'ရေကူးတတ်လား?'),
(9, 18, 'တတ်တယ်၊ ကလေးတုန်းက သင်ခဲ့တယ်။'),
(10, 19, 'basketball ကစားတယ်နော်?'),
(10, 20, 'ဟုတ်ကဲ့၊ ကျောင်းတုန်းက ကစားခဲ့တယ်။'),
(11, 1, 'iPhone 14 Pro Max ရှိသေးလား?'),
(11, 1, 'ရှိသေးတယ်။ စျေး ညှိလို့ ရတယ်။'),
(12, 2, 'Samsung Galaxy S23 ဘယ်လောက် လျှော့ပေးမလဲ?'),
(12, 2, '40000 coin ပေးရင် ရတယ်။'),
(13, 3, 'MacBook Air M2 condition ဘယ်လို ရှိလဲ?'),
(13, 3, 'condition ကောင်းတယ်၊ battery health 95% ရှိတယ်။'),
(14, 4, 'Honda Wave 125i မောင်းနှင်မှု ဘယ်လောက် ရှိပြီလဲ?'),
(14, 4, '15000 km ရှိပြီ။'),
(15, 5, 'Yamaha Exciter 150 warranty ရှိသေးလား?'),
(15, 5, 'warranty ကုန်သွားပြီ။');

-- ============================================================
-- 8. COIN TRANSACTIONS (some users have coins)
-- ============================================================
INSERT INTO ledger.coin_transactions (user_id, amount, kind, request_id) VALUES
(1, 1000, 'topup', 'seed-topup-001'),
(2, 500, 'topup', 'seed-topup-002'),
(3, 2000, 'topup', 'seed-topup-003'),
(4, 750, 'topup', 'seed-topup-004'),
(5, 1500, 'topup', 'seed-topup-005'),
(6, 300, 'topup', 'seed-topup-006'),
(7, 800, 'topup', 'seed-topup-007'),
(8, 1200, 'topup', 'seed-topup-008'),
(9, 600, 'topup', 'seed-topup-009'),
(10, 900, 'topup', 'seed-topup-010');

COMMIT;

-- ============================================================
-- Summary
-- ============================================================
-- Users: 50
-- User Profiles: 50
-- Friendships: 80
-- Posts: 100
-- Reactions: 30
-- Comments: 8
-- Market Products: 50
-- Dating Profiles: 20
-- Swipes: 10
-- Matches: 3
-- Chat Threads: 20
-- Chat Messages: 40
-- Coin Transactions: 10
