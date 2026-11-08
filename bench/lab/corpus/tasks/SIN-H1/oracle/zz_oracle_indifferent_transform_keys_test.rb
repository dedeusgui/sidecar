# frozen_string_literal: true

require 'minitest/autorun'
require 'sinatra/indifferent_hash'

# Oculto: copiado para test/ somente após o agente terminar. Verifica comportamento observável.
class ZzSinatraIndifferentTransformKeysOracleTest < Minitest::Test
  IH = Sinatra::IndifferentHash

  def test_mapping_matches_string_and_symbol_keys
    result = IH[a: 1].transform_keys(a: :z)
    assert_equal({ 'z' => 1 }, result)
    assert_instance_of IH, result
  end

  def test_string_mapping_keys_also_match
    assert_equal({ 'z' => 1 }, IH[a: 1].transform_keys('a' => :z))
  end

  def test_mapping_and_block_together
    result = IH[a: 1, b: 2].transform_keys(a: :z) { |k| k.upcase }
    assert_equal({ 'z' => 1, 'B' => 2 }, result)
  end

  def test_unmapped_keys_keep_their_name_without_block
    assert_equal({ 'z' => 1, 'b' => 2 }, IH[a: 1, b: 2].transform_keys(a: :z))
  end

  def test_output_keys_are_strings_and_indifferent
    result = IH[a: 1].transform_keys(a: :z)
    assert(result.keys.all? { |k| k.is_a?(String) })
    assert_equal 1, result[:z]
    assert_equal 1, result['z']
  end

  def test_non_symbol_keys_in_mapping
    assert_equal({ 'y' => 'x' }, IH[1 => 'x'].transform_keys(1 => :y))
  end

  def test_mapping_value_given_as_string
    assert_equal({ 'z' => 1 }, IH[a: 1].transform_keys(a: 'z'))
  end

  def test_mapping_with_missing_key_is_ignored
    assert_equal({ 'a' => 1 }, IH[a: 1].transform_keys(q: :z))
  end

  def test_values_are_not_changed_by_key_transform
    assert_equal :x, IH[a: :x].transform_keys(a: :z)['z']
  end

  def test_nested_hash_value_stays_indifferent
    assert_equal 1, IH[a: { b: 1 }].transform_keys(a: :z)['z'][:b]
  end

  def test_bang_mapping_mutates_and_returns_self
    hash = IH[a: 1, b: 2]
    result = hash.transform_keys!(a: :z)
    assert_same hash, result
    assert_equal({ 'z' => 1, 'b' => 2 }, hash)
  end

  def test_non_bang_mapping_does_not_mutate_receiver
    hash = IH[a: 1]
    hash.transform_keys(a: :z)
    assert_equal({ 'a' => 1 }, hash)
  end

  def test_block_only_transform_keys_unchanged
    assert_equal({ 'aa' => 1 }, IH[a: 1].transform_keys { |k| k.to_s * 2 })
  end

  def test_block_only_transform_keys_bang_unchanged
    hash = IH[a: 1]
    hash.transform_keys! { |_k| :b }
    assert_equal({ 'b' => 1 }, hash)
  end

  def test_block_returning_symbol_becomes_string_key
    assert_equal({ 'ax' => 1 }, IH[a: 1].transform_keys { |k| :"#{k}x" })
  end
end
