-- 0002 시드의 원래 별칭으로 되돌린다.
UPDATE tags SET aliases = '["데브옵스","ci/cd","docker","도커","인프라","infra"]' WHERE name = 'devops';
UPDATE tags SET aliases = '["백엔드","서버","server","api"]' WHERE name = 'backend';
