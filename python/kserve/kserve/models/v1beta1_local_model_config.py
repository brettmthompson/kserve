# Copyright 2023 The KServe Authors.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#    http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# coding: utf-8

"""
    KServe

    Python SDK for KServe  # noqa: E501

    The version of the OpenAPI document: v0.1
    Generated by: https://openapi-generator.tech
"""


import pprint
import re  # noqa: F401

import six

from kserve.configuration import Configuration


class V1beta1LocalModelConfig(object):
    """NOTE: This class is auto generated by OpenAPI Generator.
    Ref: https://openapi-generator.tech

    Do not edit the class manually.
    """

    """
    Attributes:
      openapi_types (dict): The key is attribute name
                            and the value is attribute type.
      attribute_map (dict): The key is attribute name
                            and the value is json key in definition.
    """
    openapi_types = {
        'default_job_image': 'str',
        'enabled': 'bool',
        'fs_group': 'int',
        'job_namespace': 'str',
        'job_ttl_seconds_after_finished': 'int',
        'reconcilation_frequency_in_secs': 'int'
    }

    attribute_map = {
        'default_job_image': 'defaultJobImage',
        'enabled': 'enabled',
        'fs_group': 'fsGroup',
        'job_namespace': 'jobNamespace',
        'job_ttl_seconds_after_finished': 'jobTTLSecondsAfterFinished',
        'reconcilation_frequency_in_secs': 'reconcilationFrequencyInSecs'
    }

    def __init__(self, default_job_image=None, enabled=False, fs_group=None, job_namespace='', job_ttl_seconds_after_finished=None, reconcilation_frequency_in_secs=None, local_vars_configuration=None):  # noqa: E501
        """V1beta1LocalModelConfig - a model defined in OpenAPI"""  # noqa: E501
        if local_vars_configuration is None:
            local_vars_configuration = Configuration()
        self.local_vars_configuration = local_vars_configuration

        self._default_job_image = None
        self._enabled = None
        self._fs_group = None
        self._job_namespace = None
        self._job_ttl_seconds_after_finished = None
        self._reconcilation_frequency_in_secs = None
        self.discriminator = None

        if default_job_image is not None:
            self.default_job_image = default_job_image
        self.enabled = enabled
        if fs_group is not None:
            self.fs_group = fs_group
        self.job_namespace = job_namespace
        if job_ttl_seconds_after_finished is not None:
            self.job_ttl_seconds_after_finished = job_ttl_seconds_after_finished
        if reconcilation_frequency_in_secs is not None:
            self.reconcilation_frequency_in_secs = reconcilation_frequency_in_secs

    @property
    def default_job_image(self):
        """Gets the default_job_image of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The default_job_image of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: str
        """
        return self._default_job_image

    @default_job_image.setter
    def default_job_image(self, default_job_image):
        """Sets the default_job_image of this V1beta1LocalModelConfig.


        :param default_job_image: The default_job_image of this V1beta1LocalModelConfig.  # noqa: E501
        :type: str
        """

        self._default_job_image = default_job_image

    @property
    def enabled(self):
        """Gets the enabled of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The enabled of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: bool
        """
        return self._enabled

    @enabled.setter
    def enabled(self, enabled):
        """Sets the enabled of this V1beta1LocalModelConfig.


        :param enabled: The enabled of this V1beta1LocalModelConfig.  # noqa: E501
        :type: bool
        """
        if self.local_vars_configuration.client_side_validation and enabled is None:  # noqa: E501
            raise ValueError("Invalid value for `enabled`, must not be `None`")  # noqa: E501

        self._enabled = enabled

    @property
    def fs_group(self):
        """Gets the fs_group of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The fs_group of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: int
        """
        return self._fs_group

    @fs_group.setter
    def fs_group(self, fs_group):
        """Sets the fs_group of this V1beta1LocalModelConfig.


        :param fs_group: The fs_group of this V1beta1LocalModelConfig.  # noqa: E501
        :type: int
        """

        self._fs_group = fs_group

    @property
    def job_namespace(self):
        """Gets the job_namespace of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The job_namespace of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: str
        """
        return self._job_namespace

    @job_namespace.setter
    def job_namespace(self, job_namespace):
        """Sets the job_namespace of this V1beta1LocalModelConfig.


        :param job_namespace: The job_namespace of this V1beta1LocalModelConfig.  # noqa: E501
        :type: str
        """
        if self.local_vars_configuration.client_side_validation and job_namespace is None:  # noqa: E501
            raise ValueError("Invalid value for `job_namespace`, must not be `None`")  # noqa: E501

        self._job_namespace = job_namespace

    @property
    def job_ttl_seconds_after_finished(self):
        """Gets the job_ttl_seconds_after_finished of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The job_ttl_seconds_after_finished of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: int
        """
        return self._job_ttl_seconds_after_finished

    @job_ttl_seconds_after_finished.setter
    def job_ttl_seconds_after_finished(self, job_ttl_seconds_after_finished):
        """Sets the job_ttl_seconds_after_finished of this V1beta1LocalModelConfig.


        :param job_ttl_seconds_after_finished: The job_ttl_seconds_after_finished of this V1beta1LocalModelConfig.  # noqa: E501
        :type: int
        """

        self._job_ttl_seconds_after_finished = job_ttl_seconds_after_finished

    @property
    def reconcilation_frequency_in_secs(self):
        """Gets the reconcilation_frequency_in_secs of this V1beta1LocalModelConfig.  # noqa: E501


        :return: The reconcilation_frequency_in_secs of this V1beta1LocalModelConfig.  # noqa: E501
        :rtype: int
        """
        return self._reconcilation_frequency_in_secs

    @reconcilation_frequency_in_secs.setter
    def reconcilation_frequency_in_secs(self, reconcilation_frequency_in_secs):
        """Sets the reconcilation_frequency_in_secs of this V1beta1LocalModelConfig.


        :param reconcilation_frequency_in_secs: The reconcilation_frequency_in_secs of this V1beta1LocalModelConfig.  # noqa: E501
        :type: int
        """

        self._reconcilation_frequency_in_secs = reconcilation_frequency_in_secs

    def to_dict(self):
        """Returns the model properties as a dict"""
        result = {}

        for attr, _ in six.iteritems(self.openapi_types):
            value = getattr(self, attr)
            if isinstance(value, list):
                result[attr] = list(map(
                    lambda x: x.to_dict() if hasattr(x, "to_dict") else x,
                    value
                ))
            elif hasattr(value, "to_dict"):
                result[attr] = value.to_dict()
            elif isinstance(value, dict):
                result[attr] = dict(map(
                    lambda item: (item[0], item[1].to_dict())
                    if hasattr(item[1], "to_dict") else item,
                    value.items()
                ))
            else:
                result[attr] = value

        return result

    def to_str(self):
        """Returns the string representation of the model"""
        return pprint.pformat(self.to_dict())

    def __repr__(self):
        """For `print` and `pprint`"""
        return self.to_str()

    def __eq__(self, other):
        """Returns true if both objects are equal"""
        if not isinstance(other, V1beta1LocalModelConfig):
            return False

        return self.to_dict() == other.to_dict()

    def __ne__(self, other):
        """Returns true if both objects are not equal"""
        if not isinstance(other, V1beta1LocalModelConfig):
            return True

        return self.to_dict() != other.to_dict()
