# Board API

The board module exposes repository selectors and paginated issue boards. It maps store models to public DTOs so webhook secret environment references never leave the API. Pull requests are linked by closing issue number and repository; pipeline runs are linked by head SHA and repository.
