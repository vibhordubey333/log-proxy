1. Basic HEAD — triggers download + cache on first hit
   curl -I http://localhost:8080/logs/lastSuccessfulBuild
2. Confirm cache landed on disk
   ls -la ./cache/
   cat ./cache/lastSuccessfulBuild.log
3. GET the whole file
   curl -s http://localhost:8080/logs/lastSuccessfulBuild
4. GET with offset
   curl -s "http://localhost:8080/logs/lastSuccessfulBuild?offset=25"
5. GET with limit
   curl -s "http://localhost:8080/logs/lastSuccessfulBuild?limit=50"
6. GET with offset + limit (middle slice)
   curl -s "http://localhost:8080/logs/lastSuccessfulBuild?offset=100&limit=50"
7. Offset beyond EOF → 416
   curl -i "http://localhost:8080/logs/lastSuccessfulBuild?offset=999999"
8. Negative offset → 400
   curl -i "http://localhost:8080/logs/lastSuccessfulBuild?offset=-5"
9. Non-integer limit → 400
   curl -i "http://localhost:8080/logs/lastSuccessfulBuild?limit=abc"
10. Invalid build_id, single path segment → 400 (our own validation) — confirmed working
    curl -i "http://localhost:8080/logs/build.1"
11. Path traversal via encoded slash → 404 (Gin's router, before our validation even runs) — confirmed as expected
    curl -i "http://localhost:8080/logs/..%2F..%2Fetc%2Fpasswd"
    Worth noting explicitly in the interview: two different layers catch two different shapes of bad input, and that's fine — just know which layer caught which when they ask.
12. Different build_id gets its own cache entry
    curl -I http://localhost:8080/logs/12345
    ls -la ./cache/
13. Concurrent requests for the same new build_id (singleflight check)
    curl -I http://localhost:8080/logs/99999 &
    curl -I http://localhost:8080/logs/99999 &
    curl -I http://localhost:8080/logs/99999 &
    wait
    ls -la ./cache/99999*