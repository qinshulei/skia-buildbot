#!/usr/bin/env python
# Copyright (c) 2012 The Chromium Authors. All rights reserved.
# Use of this source code is governed by a BSD-style license that can be
# found in the LICENSE file.

""" Prepare runtime resources that are needed by Test builders but not
    Bench builders. """

from build_step import BuildStep
from utils import file_utils
from utils import shell_utils
import build_step
import os
import shutil
import sys
import tempfile


class PreRender(BuildStep):

  def _CreateExpectationsSummaryFromImages(self, srcdir, destpath):
    """ Create GM expectations summary at destpath from the images in srcdir """
    print 'Creating GM expectations summary %s from images in %s ...' % (
        destpath, srcdir)

    if not os.path.isdir(srcdir):
      print 'image dir %s does not exist' % srcdir
      self._CreateEmptyExpectationsSummary(destpath)
      return

    # Delete any non-image files in _gm_expected_dir, because they will
    # cause skimage to fail.
    # TODO(epoger): it would be better to fix skimage so that it doesn't
    # fail in that case, but we shouldn't be calling skimage on the bots for
    # more than a week anyway...
    num_imagefiles = 0
    for filename in os.listdir(srcdir):
      if filename.endswith('.png'):
        num_imagefiles += 1
      else:
        filepath = os.path.join(srcdir, filename)
        if os.path.isfile(filepath):
          print 'Deleting nonimage file %s' % filepath
          os.remove(filepath)

    if num_imagefiles == 0:
      print 'image dir %s contains no image files' % srcdir
      self._CreateEmptyExpectationsSummary(destpath)
      return

    cmd = [self._PathToBinary('skimage'),
           '--readPath', srcdir,
           '--createExpectationsPath', destpath,
           ]
    shell_utils.Bash(cmd)

  def _CreateEmptyExpectationsSummary(self, destpath):
    print 'Creating empty GM expectations summary in file %s .' % destpath
    f = open(destpath, 'w')
    f.write("""
        {
           "actual-results" : {
             "failed" : null,
             "failure-ignored" : null,
             "no-comparison" : null,
             "succeeded" : null
           },
           "expected-results" : null
        }
""")
    f.close()

  def _Run(self):
    # Prepare directory to hold GM expectations.
    device_gm_expectations_path = self.DevicePathJoin(
        self._device_dirs.GMExpectedDir(), build_step.GM_EXPECTATIONS_FILENAME)
    self.CreateCleanDirectory(self._device_dirs.GMExpectedDir())

    # Push the GM expectations JSON file to the device.
    #
    # Soon, this will just be copying a single file, but currently the GM
    # expectations are stored as individual image files in SVN.
    # So, if the single expectations file hasn't been checked into SVN yet,
    # create it from the image files (using the skimage tool).
    #
    # We do NOT write this locally-generated expected-results.json file into
    # the SVN-managed directory; instead, we write it into the locally
    # maintained DeviceDir.GMExpectedDir().  During the transition period
    # (until Step 4 in https://goto.google.com/ChecksumTransitionDetail ,
    # when the expectations will be stored in SVN as JSON files instead of
    # individual image files), this JSON expectations file will be
    # regenerated every time the buildbot runs.
    repo_gm_expectations_path = os.path.join(
        self._gm_expected_dir, build_step.GM_EXPECTATIONS_FILENAME)
    if os.path.exists(repo_gm_expectations_path):
      print 'Pushing GM expectations from %s on host to %s on device...' % (
          repo_gm_expectations_path, device_gm_expectations_path)
      self.PushFileToDevice(repo_gm_expectations_path,
                            device_gm_expectations_path)
    else:
      tempdir = tempfile.mkdtemp()
      temp_gm_expectations_path = os.path.join(
          tempdir, build_step.GM_EXPECTATIONS_FILENAME)
      self._CreateExpectationsSummaryFromImages(
          srcdir=self._gm_expected_dir, destpath=temp_gm_expectations_path)
      print 'Pushing GM expectations from %s on host to %s on device...' % (
          temp_gm_expectations_path, device_gm_expectations_path)
      self.PushFileToDevice(temp_gm_expectations_path,
                            device_gm_expectations_path)
      shutil.rmtree(tempdir)

    # Prepare directory to hold GM actuals.
    if os.path.exists(self._gm_actual_dir):
      print 'Removing %s' % self._gm_actual_dir
      shutil.rmtree(self._gm_actual_dir)
    print 'Creating %s' % self._gm_actual_dir
    os.makedirs(self._gm_actual_dir)

    # Copy expectations file and images to decode in skimage to device.
    self.CopyDirectoryContentsToDevice(self._skimage_expected_dir,
                                       self._device_dirs.SKImageExpectedDir())

    self.CopyDirectoryContentsToDevice(self._skimage_in_dir,
                                       self._device_dirs.SKImageInDir())

    # Create an out directory locally for android builds, so the files can be
    # copied back to the master
    file_utils.CreateCleanLocalDir(self._skimage_out_dir)

    # Create a directory for the output of skimage
    self.CreateCleanDirectory(self._device_dirs.SKImageOutDir())


if '__main__' == __name__:
  sys.exit(BuildStep.RunBuildStep(PreRender))
