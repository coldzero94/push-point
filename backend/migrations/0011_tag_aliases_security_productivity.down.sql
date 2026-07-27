-- 0002 시드의 원래 별칭으로 되돌린다.
UPDATE tags SET aliases = '["보안","시큐리티","해킹","hacking"]' WHERE name = 'security';
UPDATE tags SET aliases = '["생산성","워크플로우","workflow","도구","tools"]' WHERE name = 'productivity';
