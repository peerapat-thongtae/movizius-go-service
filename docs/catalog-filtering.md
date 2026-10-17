# Catalog filtering rules

กติกานี้ใช้กับ Movie และ TV (ไม่รวม Person) เพื่อให้ title ที่ส่งออกจาก Go
service มีอยู่ใน local catalog และ user-state ถูกคำนวณจากข้อมูลที่ถูกต้อง

## กติกากลาง

1. Local DB เป็น source of truth ของ title ที่แสดงได้
   - TMDB ใช้หา candidate หรือ enrich metadata ได้เท่านั้น
   - ก่อน return title ต้องผ่าน `FindByTMDBIDs`/DB gate
   - ไม่ fetch TMDB detail ให้ ID ที่ไม่พบใน local catalog

2. Endpoint ที่มี user-selectable pagination ต้องเริ่มจาก DB
   - Discover, Search และ Library ใช้ local IDs, filters, sort และ totals

3. Endpoint ที่ไม่มี pagination ใช้ TMDB เป็น candidate source ได้
   - Hero, Trending rail, Reels, random/recommendation fallback และ Collection
   - ต้อง filter candidate IDs ผ่าน DB ก่อน enrich/return
   - เมื่อผลหลัง filter ไม่พอ ให้ดึง candidate หน้าถัดไปตามความเหมาะสม

4. Detail gate
   - Movie/TV detail และ TV season ตอบ 404 เมื่อ title หลักไม่มีใน local DB
   - Collection parts แสดงเฉพาะ movie ที่ผ่าน DB gate

## User-status filtering

User status ต้อง filter ใน Go service ไม่ใช่ frontend เพราะ TV status ต้องอาศัย
episode history และข้อมูล series ใน DB

| Surface | Movie ที่ตัดออก | TV ที่ตัดออก |
| --- | --- | --- |
| Home Hero | `watched` | `watched`, `waiting_next_ep` |
| Reels external sources | `watched`, `watchlist` | - |
| Discover / Library | ตาม request filter | ตาม request filter |
| Detail | ไม่ตัด | ไม่ตัด |

`waiting_next_ep` ต้องใช้ status ที่คำนวณจาก `episode_watched`,
`last_episode_to_air` และ `next_episode_to_air`; ห้ามอนุมานจาก raw `watched_at`
อย่างเดียว

## Home Hero contract

- `GET /movie/hero`, `GET /tv/hero`
- ไม่มี pagination
- แต่ละ media type พยายามคืน 20 titles
- สแกน TMDB Trending ได้สูงสุด 25 หน้าเพื่อเติมหลัง DB/status/backdrop filter
- ต้องผ่าน DB gate, acceptability rules, status rules และมี backdrop
- อาจคืนต่ำกว่า 20 ได้เฉพาะเมื่อไม่มี title ที่ผ่านกติกาเพียงพอใน source ที่สแกน

## Checklist สำหรับ endpoint list ใหม่

- [ ] มี pagination ที่ผู้ใช้เลือกหน้าเองหรือไม่? ถ้ามี ให้ query DB ก่อน
- [ ] TMDB candidate ถูก DB gate ก่อน TMDB detail และก่อน return หรือไม่?
- [ ] ต้อง exclude user status ใด และต้องใช้ calculated TV status หรือไม่?
- [ ] ภาพที่ UI ต้องใช้ เช่น backdrop/poster ถูกตรวจตาม surface หรือไม่?
- [ ] ผลหลัง filter ไม่พอ ต้อง backfill จากหน้า/source ถัดไปหรือไม่?
