# frozen_string_literal: true

require 'minitest/autorun'
require 'rack/mock'
require 'sinatra/base'

# Oculto: copiado para test/ somente após o agente terminar. Verifica cabeçalhos HTTP observáveis.
class ZzSinatraCacheControlOracleTest < Minitest::Test
  class App < Sinatra::Base
    get('/nil-value') { cache_control :public, max_age: nil; 'x' }
    get('/nil-several') { cache_control max_age: nil, s_maxage: nil, public: true; 'x' }
    get('/nil-only') { cache_control max_age: nil; 'x' }
    get('/nil-generic') { cache_control :public, foo_bar: nil; 'x' }
    get('/zero-kept') { cache_control :public, max_age: 0; 'x' }
    get('/string-max-age') { cache_control max_age: '120'; 'x' }
    get('/booleans') { cache_control :public, false_thing: false, x_y: true; 'x' }
    get('/s-maxage-float') { cache_control :no_store, s_maxage: 60.9; 'x' }
    get('/order') { expires 500, :public, :must_revalidate; 'x' }
    get('/past-time') { expires(Time.at(0), :public); 'x' }
    get('/past-time-explicit') { expires(Time.at(0), :public, max_age: 30); 'x' }
    get('/zero-seconds') { expires 0, :public; 'x' }
    get('/negative-seconds') { expires(-60, :public); 'x' }
    get('/future-time') { expires(Time.now + 3600, :public); 'x' }
    get('/no-values') { cache_control max_age: nil; 'x' }
  end

  def get(path)
    Rack::MockRequest.new(App).get(path)
  end

  def test_nil_option_is_omitted
    assert_equal 'public', get('/nil-value').headers['Cache-Control']
  end

  def test_several_nil_options_are_omitted
    assert_equal 'public', get('/nil-several').headers['Cache-Control']
  end

  def test_nil_generic_option_is_omitted
    assert_equal 'public', get('/nil-generic').headers['Cache-Control']
  end

  def test_only_nil_options_sets_no_header
    assert_nil get('/nil-only').headers['Cache-Control']
  end

  def test_zero_max_age_is_kept
    assert_equal 'public, max-age=0', get('/zero-kept').headers['Cache-Control']
  end

  def test_string_max_age_becomes_integer
    assert_equal 'max-age=120', get('/string-max-age').headers['Cache-Control']
  end

  def test_false_removed_true_bare_and_underscores_dashed
    assert_equal 'public, x-y', get('/booleans').headers['Cache-Control']
  end

  def test_s_maxage_float_truncated_and_dashed
    assert_equal 'no-store, s-maxage=60', get('/s-maxage-float').headers['Cache-Control']
  end

  def test_directive_order_bare_values_first
    res = get('/order')
    assert_equal 'public, must-revalidate, max-age=500', res.headers['Cache-Control']
  end

  def test_past_time_gives_zero_max_age_and_keeps_expires
    res = get('/past-time')
    assert_equal 'public, max-age=0', res.headers['Cache-Control']
    assert_equal 'Thu, 01 Jan 1970 00:00:00 GMT', res.headers['Expires']
  end

  def test_past_time_with_explicit_max_age_keeps_explicit
    assert_equal 'public, max-age=30', get('/past-time-explicit').headers['Cache-Control']
  end

  def test_zero_seconds_gives_zero_max_age
    assert_equal 'public, max-age=0', get('/zero-seconds').headers['Cache-Control']
  end

  def test_negative_seconds_never_negative_max_age
    res = get('/negative-seconds')
    assert_equal 'public, max-age=0', res.headers['Cache-Control']
    expires = Time.httpdate(res.headers['Expires'])
    assert_in_delta Time.now.to_i - 60, expires.to_i, 3
  end

  def test_future_time_keeps_remaining_seconds
    max_age = get('/future-time').headers['Cache-Control'][/max-age=(\d+)/, 1].to_i
    assert_includes 3598..3600, max_age
  end
end
