import { getFormat } from './editor';
import { Format } from 'types';

describe('getFormat', () => {
  describe('AUTO mode detection', () => {
    it('returns TIMESERIES when first field has "as time" alias and >= 2 fields', () => {
      expect(getFormat('SELECT ts as time, value FROM t', Format.AUTO)).toBe(Format.TIMESERIES);
    });

    it('returns TIMESERIES with timestamp alias as time', () => {
      expect(getFormat('SELECT timestamp as time, count FROM t', Format.AUTO)).toBe(Format.TIMESERIES);
    });

    it('returns TIMESERIES case-insensitively for "as time"', () => {
      expect(getFormat('SELECT ts AS TIME, value FROM t', Format.AUTO)).toBe(Format.TIMESERIES);
    });

    it('returns TABLE when no "as time" alias', () => {
      expect(getFormat('SELECT a, b FROM t', Format.AUTO)).toBe(Format.TABLE);
    });

    it('returns TABLE when field has no alias', () => {
      expect(getFormat('SELECT ts, value FROM t', Format.AUTO)).toBe(Format.TABLE);
    });

    it('returns TABLE when only one field even with "as time" alias', () => {
      expect(getFormat('SELECT ts as time FROM t', Format.AUTO)).toBe(Format.TABLE);
    });

    it('returns TABLE for empty SQL', () => {
      expect(getFormat('', Format.AUTO)).toBe(Format.TABLE);
    });

    it('returns TABLE for unparseable SQL', () => {
      expect(getFormat('NOT VALID SQL', Format.AUTO)).toBe(Format.TABLE);
    });
  });

  describe('explicit format overrides', () => {
    it('returns TIMESERIES when selectedFormat is TIMESERIES regardless of SQL', () => {
      expect(getFormat('SELECT a, b FROM t', Format.TIMESERIES)).toBe(Format.TIMESERIES);
    });

    it('returns TABLE when selectedFormat is TABLE regardless of SQL', () => {
      expect(getFormat('SELECT ts as time, value FROM t', Format.TABLE)).toBe(Format.TABLE);
    });

    it('returns TABLE when selectedFormat is TABLE for empty SQL', () => {
      expect(getFormat('', Format.TABLE)).toBe(Format.TABLE);
    });

    it('returns TIMESERIES when selectedFormat is TIMESERIES for empty SQL', () => {
      expect(getFormat('', Format.TIMESERIES)).toBe(Format.TIMESERIES);
    });
  });
});
