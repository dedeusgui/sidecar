# frozen_string_literal: true

require 'minitest/autorun'
require 'rack/mock'
require 'sinatra/base'

# Oculto: copiado para test/ somente após o agente terminar. Verifica comportamento HTTP observável.
class ZzSinatraEtagWeakOracleTest < Minitest::Test
  class App < Sinatra::Base
    get('/strong') { etag 'abc'; 'strong-body' }
    get('/weak') { etag 'abc', :weak; 'weak-body' }
    get('/strong-if-match') { etag 'abc'; 'if-match-body' }
  end

  def get(path, headers = {})
    Rack::MockRequest.new(App).get(path, headers)
  end

  def test_if_none_match_weak_comparison_strong_etag
    res = get('/strong', 'HTTP_IF_NONE_MATCH' => 'W/"abc"')
    assert_equal 304, res.status, 'If-None-Match usa comparação fraca: W/"abc" equivale a "abc"'
  end

  def test_if_none_match_weak_comparison_weak_etag
    res = get('/weak', 'HTTP_IF_NONE_MATCH' => '"abc"')
    assert_equal 304, res.status, 'If-None-Match usa comparação fraca: "abc" equivale a W/"abc"'
  end

  def test_if_none_match_weak_both_sides
    res = get('/weak', 'HTTP_IF_NONE_MATCH' => 'W/"abc"')
    assert_equal 304, res.status
  end

  def test_if_none_match_list_with_weak_member
    res = get('/strong', 'HTTP_IF_NONE_MATCH' => '"xyz", W/"abc"')
    assert_equal 304, res.status
  end

  def test_if_none_match_mismatch_still_returns_body
    res = get('/strong', 'HTTP_IF_NONE_MATCH' => 'W/"xyz"')
    assert_equal 200, res.status
    assert_equal 'strong-body', res.body
  end

  def test_if_match_keeps_matching_tag_success
    res = get('/strong-if-match', 'HTTP_IF_MATCH' => '"abc"')
    assert_equal 200, res.status
    assert_equal 'if-match-body', res.body
  end

  def test_if_match_keeps_mismatch_precondition_failed
    res = get('/strong-if-match', 'HTTP_IF_MATCH' => '"xyz"')
    assert_equal 412, res.status
  end
end
