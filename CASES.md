# Conformance cases and their tests

Contract kit 3.0.0-rc.6 (`testdata/conformance/`). Every case of `conformance.md` runs as its own
subtest of `TestConformance`; a case without a test fails that test. The W cases run in the `webhook`
package, all others in the root package. D-10 runs one subtest per matrix sub-case (256).

Run one case: `go test -run 'TestConformance/S-16' .` (or `./webhook` for W cases).

| Case | Title | Test file | Package and test |
|---|---|---|---|
| V-01 | Known constants equal vocab.json | `conformance_decode_test.go` | `.` `TestConformance/V-01_Known_constants_equal_vocab.json` |
| D-01 | Full article: every include, every entitlement | `conformance_decode_test.go` | `.` `TestConformance/D-01_Full_article:_every_include,_every_entitlement` |
| D-02 | Raw skeleton (raw channel ignores include and entitlements) | `conformance_decode_test.go` | `.` `TestConformance/D-02_Raw_skeleton_(raw_channel_ignores_include_and_entitlements)` |
| D-03 | No include, every entitlement | `conformance_decode_test.go` | `.` `TestConformance/D-03_No_include,_every_entitlement` |
| D-04 | Every include requested, no entitlement | `conformance_decode_test.go` | `.` `TestConformance/D-04_Every_include_requested,_no_entitlement` |
| D-05 | events requested and entitled, companies not requested | `conformance_decode_test.go` | `.` `TestConformance/D-05_events_requested_and_entitled,_companies_not_requested` |
| D-06 | events and companies requested, entityAccess missing | `conformance_decode_test.go` | `.` `TestConformance/D-06_events_and_companies_requested,_entityAccess_missing` |
| D-07 | Legacy values delivered as they are | `conformance_decode_test.go` | `.` `TestConformance/D-07_Legacy_values_delivered_as_they_are` |
| D-08 | Non-ASCII text | `conformance_decode_test.go` | `.` `TestConformance/D-08_Non-ASCII_text` |
| D-09 | Sentiment with a null label, event with null direction/importance/confidence, companies [] | `conformance_decode_test.go` | `.` `TestConformance/D-09_Sentiment_with_a_null_label,_event_with_null_direction/importance/confidence,_companies_[]` |
| D-10 | Decode matrix (include x entitlements x enriched/raw) | `conformance_decode_test.go` | `.` `TestConformance/D-10_Decode_matrix_(include_x_entitlements_x_enriched/raw)` |
| D-11 | Blank summary and content arrive as null | `conformance_decode_test.go` | `.` `TestConformance/D-11_Blank_summary_and_content_arrive_as_null` |
| F-01 | Unknown fields at every level of the article | `conformance_decode_test.go` | `.` `TestConformance/F-01_Unknown_fields_at_every_level_of_the_article` |
| F-02 | Unknown values in every open response enum | `conformance_decode_test.go` | `.` `TestConformance/F-02_Unknown_values_in_every_open_response_enum` |
| F-03 | New keys on REST, webhook and WebSocket envelopes | `conformance_decode_test.go` | `.` `TestConformance/F-03_New_keys_on_REST,_webhook_and_WebSocket_envelopes` |
| F-04 | Unknown webhook event name, unknown problem code, unknown 429 limitType | `conformance_decode_test.go` | `.` `TestConformance/F-04_Unknown_webhook_event_name,_unknown_problem_code,_unknown_429_limitType` |
| R-01 | articles.search with one parameter sends exactly that parameter | `conformance_rest_test.go` | `.` `TestConformance/R-01_articles.search_with_one_parameter_sends_exactly_that_parameter` |
| R-02 | articles.search with every parameter | `conformance_rest_test.go` | `.` `TestConformance/R-02_articles.search_with_every_parameter` |
| R-03 | articles.search without parameters | `conformance_rest_test.go` | `.` `TestConformance/R-03_articles.search_without_parameters` |
| R-04 | articles.get with include, comma-joined | `conformance_rest_test.go` | `.` `TestConformance/R-04_articles.get_with_include,_comma-joined` |
| R-05 | articles.getByLink URL-encodes the link | `conformance_rest_test.go` | `.` `TestConformance/R-05_articles.getByLink_URL-encodes_the_link` |
| R-06 | companies.search with q and mic | `conformance_rest_test.go` | `.` `TestConformance/R-06_companies.search_with_q_and_mic` |
| R-07 | companies.getMany joins ids with commas | `conformance_rest_test.go` | `.` `TestConformance/R-07_companies.getMany_joins_ids_with_commas` |
| R-08 | companies.get returns ETag and Cache-Control | `conformance_rest_test.go` | `.` `TestConformance/R-08_companies.get_returns_ETag_and_Cache-Control` |
| R-09 | companies.get with If-None-Match answers 304 | `conformance_rest_test.go` | `.` `TestConformance/R-09_companies.get_with_If-None-Match_answers_304` |
| R-10 | sources.list with fullArticleAccess | `conformance_rest_test.go` | `.` `TestConformance/R-10_sources.list_with_fullArticleAccess` |
| R-11 | sources.list without fullArticleAccess | `conformance_rest_test.go` | `.` `TestConformance/R-11_sources.list_without_fullArticleAccess` |
| R-12 | An unknown request key is dropped silently by the server | `conformance_rest_test.go` | `.` `TestConformance/R-12_An_unknown_request_key_is_dropped_silently_by_the_server` |
| R-13 | WebSocket subscribe frame, enriched | `conformance_stream_test.go` | `.` `TestConformance/R-13_WebSocket_subscribe_frame,_enriched` |
| R-14 | WebSocket subscribe frame, raw | `conformance_stream_test.go` | `.` `TestConformance/R-14_WebSocket_subscribe_frame,_raw` |
| P-01 | Three-page cursor walk | `conformance_rest_test.go` | `.` `TestConformance/P-01_Three-page_cursor_walk` |
| P-02 | limit and include may change between pages | `conformance_rest_test.go` | `.` `TestConformance/P-02_limit_and_include_may_change_between_pages` |
| P-03 | Adding "to" on page 2 breaks the cursor | `conformance_rest_test.go` | `.` `TestConformance/P-03_Adding_"to"_on_page_2_breaks_the_cursor` |
| P-04 | Adding the default languages ["en"] on page 2 breaks the cursor | `conformance_rest_test.go` | `.` `TestConformance/P-04_Adding_the_default_languages_["en"]_on_page_2_breaks_the_cursor` |
| P-05 | Changing order on page 2 | `conformance_rest_test.go` | `.` `TestConformance/P-05_Changing_order_on_page_2` |
| P-06 | Reordering list values keeps the cursor valid | `conformance_rest_test.go` | `.` `TestConformance/P-06_Reordering_list_values_keeps_the_cursor_valid` |
| E-01 | Unknown enum value | `conformance_rest_test.go` | `.` `TestConformance/E-01_Unknown_enum_value` |
| E-02 | Several invalid fields at once | `conformance_rest_test.go` | `.` `TestConformance/E-02_Several_invalid_fields_at_once` |
| E-03 | Unknown field in the query language | `conformance_rest_test.go` | `.` `TestConformance/E-03_Unknown_field_in_the_query_language` |
| E-04 | Empty languages array | `conformance_rest_test.go` | `.` `TestConformance/E-04_Empty_languages_array` |
| E-05 | Unknown include value on a GET lookup | `conformance_rest_test.go` | `.` `TestConformance/E-05_Unknown_include_value_on_a_GET_lookup` |
| E-06 | companies.search without q and ids | `conformance_rest_test.go` | `.` `TestConformance/E-06_companies.search_without_q_and_ids` |
| E-07 | companies.search with q and ids | `conformance_rest_test.go` | `.` `TestConformance/E-07_companies.search_with_q_and_ids` |
| E-08 | Non-integer company id in ids | `conformance_rest_test.go` | `.` `TestConformance/E-08_Non-integer_company_id_in_ids` |
| E-09 | Non-integer companyId in the path | `conformance_rest_test.go` | `.` `TestConformance/E-09_Non-integer_companyId_in_the_path` |
| E-10 | Cursor that is not a cursor | `conformance_rest_test.go` | `.` `TestConformance/E-10_Cursor_that_is_not_a_cursor` |
| E-11 | Cursor of an unsupported version | `conformance_rest_test.go` | `.` `TestConformance/E-11_Cursor_of_an_unsupported_version` |
| E-12 | No API key | `conformance_rest_test.go` | `.` `TestConformance/E-12_No_API_key` |
| E-13 | Unknown API key | `conformance_rest_test.go` | `.` `TestConformance/E-13_Unknown_API_key` |
| E-14 | Company lookup without entityAccess | `conformance_rest_test.go` | `.` `TestConformance/E-14_Company_lookup_without_entityAccess` |
| E-15 | Unknown article id | `conformance_rest_test.go` | `.` `TestConformance/E-15_Unknown_article_id` |
| E-16 | Unknown article link | `conformance_rest_test.go` | `.` `TestConformance/E-16_Unknown_article_link` |
| E-17 | Unknown company id | `conformance_rest_test.go` | `.` `TestConformance/E-17_Unknown_company_id` |
| E-18 | Monthly token limit reached | `conformance_rest_test.go` | `.` `TestConformance/E-18_Monthly_token_limit_reached` |
| E-19 | Burst limit reached | `conformance_rest_test.go` | `.` `TestConformance/E-19_Burst_limit_reached` |
| E-20 | Server failure | `conformance_rest_test.go` | `.` `TestConformance/E-20_Server_failure` |
| E-21 | Malformed JSON body (NestJS fallback form) | `conformance_rest_test.go` | `.` `TestConformance/E-21_Malformed_JSON_body_(NestJS_fallback_form)` |
| E-22 | Unknown route under /v3 (NestJS fallback form) | `conformance_rest_test.go` | `.` `TestConformance/E-22_Unknown_route_under_/v3_(NestJS_fallback_form)` |
| E-23 | No X-Request-Id sent: the server generates one | `conformance_rest_test.go` | `.` `TestConformance/E-23_No_X-Request-Id_sent:_the_server_generates_one` |
| E-24 | Non-JSON error body | `conformance_rest_test.go` | `.` `TestConformance/E-24_Non-JSON_error_body` |
| E-25 | Cursor issued for another sort order | `conformance_rest_test.go` | `.` `TestConformance/E-25_Cursor_issued_for_another_sort_order` |
| W-01 | article.created, enriched, every include, non-ASCII body | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-01_article.created,_enriched,_every_include,_non-ASCII_body` |
| W-02 | article.updated with changedFields | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-02_article.updated_with_changedFields` |
| W-03 | article.updated for a plan without fullArticleAccess | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-03_article.updated_for_a_plan_without_fullArticleAccess` |
| W-04 | article.created on a raw webhook | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-04_article.created_on_a_raw_webhook` |
| W-05 | Dashboard test webhook (evt_test…) | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-05_Dashboard_test_webhook_(evt_test…)` |
| W-06 | v2 body is reported as wrong payload version | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-06_v2_body_is_reported_as_wrong_payload_version` |
| W-07 | Body changed after signing | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-07_Body_changed_after_signing` |
| W-08 | Timestamp older than the tolerance | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-08_Timestamp_older_than_the_tolerance` |
| W-09 | Timestamp further in the future than the tolerance | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-09_Timestamp_further_in_the_future_than_the_tolerance` |
| W-10 | Timestamp header is not ISO 8601 | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-10_Timestamp_header_is_not_ISO_8601` |
| W-11 | Timestamp in unix seconds instead of ISO 8601 | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-11_Timestamp_in_unix_seconds_instead_of_ISO_8601` |
| W-12 | No X-Webhook-Timestamp header | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-12_No_X-Webhook-Timestamp_header` |
| W-13 | No X-Webhook-Signature header | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-13_No_X-Webhook-Signature_header` |
| W-14 | Signature without the sha256= prefix | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-14_Signature_without_the_sha256=_prefix` |
| W-15 | Signature computed with the hex-decoded secret | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-15_Signature_computed_with_the_hex-decoded_secret` |
| W-16 | Case-insensitive header lookup | `webhook/conformance_test.go` | `./webhook` `TestConformance/W-16_Case-insensitive_header_lookup` |
| S-01 | Ping before subscribe gets a v2 pong; after the v3 subscribe pongs are v3 | `conformance_stream_test.go` | `.` `TestConformance/S-01_Ping_before_subscribe_gets_a_v2_pong;_after_the_v3_subscribe_pongs_are_v3` |
| S-02 | Lowercase x-api-key on the wire | `conformance_stream_test.go` | `.` `TestConformance/S-02_Lowercase_x-api-key_on_the_wire` |
| S-03 | connection.admitted echoes the nonce; a re-subscribe re-admits only with the same nonce | `conformance_stream_test.go` | `.` `TestConformance/S-03_connection.admitted_echoes_the_nonce;_a_re-subscribe_re-admits_only_with_the_same_nonce` |
| S-04 | article.created then article.updated for the same id, then an exact duplicate | `conformance_stream_test.go` | `.` `TestConformance/S-04_article.created_then_article.updated_for_the_same_id,_then_an_exact_duplicate` |
| S-05 | An unknown event name arrives | `conformance_stream_test.go` | `.` `TestConformance/S-05_An_unknown_event_name_arrives` |
| S-06 | connection.error PING_ON_DEFAULT_ROUTE | `conformance_stream_test.go` | `.` `TestConformance/S-06_connection.error_PING_ON_DEFAULT_ROUTE` |
| S-07 | VALIDATION_FAILED for a v2 key in a v3 subscribe | `conformance_stream_test.go` | `.` `TestConformance/S-07_VALIDATION_FAILED_for_a_v2_key_in_a_v3_subscribe` |
| S-08 | VALIDATION_FAILED for include on raw | `conformance_stream_test.go` | `.` `TestConformance/S-08_VALIDATION_FAILED_for_include_on_raw` |
| S-09 | connection.preempted, reason connection_limit (another connection took over) | `conformance_stream_test.go` | `.` `TestConformance/S-09_connection.preempted,_reason_connection_limit_(another_connection_took_over)` |
| S-10 | connection.preempted, reason quota_exceeded | `conformance_stream_test.go` | `.` `TestConformance/S-10_connection.preempted,_reason_quota_exceeded` |
| S-11 | Preempted before the first subscribe: the frame comes in v2 form | `conformance_stream_test.go` | `.` `TestConformance/S-11_Preempted_before_the_first_subscribe:_the_frame_comes_in_v2_form` |
| S-12 | Error for an unreadable message comes in v2 form | `conformance_stream_test.go` | `.` `TestConformance/S-12_Error_for_an_unreadable_message_comes_in_v2_form` |
| S-13 | connection.error HANDSHAKE_FAILED | `conformance_stream_test.go` | `.` `TestConformance/S-13_connection.error_HANDSHAKE_FAILED` |
| S-14 | connection.error SEARCH_FAILED | `conformance_stream_test.go` | `.` `TestConformance/S-14_connection.error_SEARCH_FAILED` |
| S-15 | connection.error INTERNAL_ERROR | `conformance_stream_test.go` | `.` `TestConformance/S-15_connection.error_INTERNAL_ERROR` |
| S-16 | No pong for 60 s | `conformance_stream_test.go` | `.` `TestConformance/S-16_No_pong_for_60_s` |
| S-17 | Raw stream: admitted, no initial push, skeleton frames | `conformance_stream_test.go` | `.` `TestConformance/S-17_Raw_stream:_admitted,_no_initial_push,_skeleton_frames` |
| S-18 | Proactive rotation before the 2 h connection lifetime | `conformance_stream_test.go` | `.` `TestConformance/S-18_Proactive_rotation_before_the_2_h_connection_lifetime` |
| S-19 | Dial refused: no API key | `conformance_stream_test.go` | `.` `TestConformance/S-19_Dial_refused:_no_API_key` |
| S-20 | Dial refused: invalid API key | `conformance_stream_test.go` | `.` `TestConformance/S-20_Dial_refused:_invalid_API_key` |
| S-21 | Dial refused: API key in the wrong format | `conformance_stream_test.go` | `.` `TestConformance/S-21_Dial_refused:_API_key_in_the_wrong_format` |
| S-22 | Dial refused: plan without WebSockets | `conformance_stream_test.go` | `.` `TestConformance/S-22_Dial_refused:_plan_without_WebSockets` |
| S-23 | Dial refused: connection limit reached, no takeover | `conformance_stream_test.go` | `.` `TestConformance/S-23_Dial_refused:_connection_limit_reached,_no_takeover` |
| S-24 | Dial refused: WebSocket quota exhausted | `conformance_stream_test.go` | `.` `TestConformance/S-24_Dial_refused:_WebSocket_quota_exhausted` |
| S-25 | Dial refused: lost the admit race | `conformance_stream_test.go` | `.` `TestConformance/S-25_Dial_refused:_lost_the_admit_race` |
| S-26 | Dial refused: server error | `conformance_stream_test.go` | `.` `TestConformance/S-26_Dial_refused:_server_error` |
| S-27 | connection.preempted, reason quota_exceeded, with quotaReconnect turned off | `conformance_stream_test.go` | `.` `TestConformance/S-27_connection.preempted,_reason_quota_exceeded,_with_quotaReconnect_turned_off` |
| S-28 | Dial refused: WebSocket quota exhausted, with quotaReconnect turned off | `conformance_stream_test.go` | `.` `TestConformance/S-28_Dial_refused:_WebSocket_quota_exhausted,_with_quotaReconnect_turned_off` |
| S-29 | VALIDATION_FAILED for an unknown sourceTypes value | `conformance_stream_test.go` | `.` `TestConformance/S-29_VALIDATION_FAILED_for_an_unknown_sourceTypes_value` |
